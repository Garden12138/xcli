#!/usr/bin/env python3
"""Offline mTLS host-bridge acceptance; never mounts or modifies live app state.

Run using the Contribution OS development Python. --source is an isolated source
copy containing the bridge, --binary is a local xcli build, and --docker-image
optionally names a pre-existing trusted dependency image by immutable image ID.
Only temporary synthetic certificates, token and copied app source enter a probe
container; it gets no host socket, account credentials or published ports.
"""
from __future__ import annotations

import argparse
from collections import Counter
import hashlib
import http.server
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import sys
import tempfile
import threading

from integration_workspace import validate_python_runtime
from native_consumers import Fixture, execute

PROBE = Path(__file__).with_name('probes') / 'contribution_os_bridge.py'


def run(args):
    source = args.source.resolve()
    validate_python_runtime(source)
    sys.path.insert(0, str(source))
    from tests.xcli_bridge_fixtures import credentials, private_write, relay_config, running_relay, INSTANCE
    from app.providers.xcli_bridge import certificate_fingerprint
    report = {'evidence': 'synthetic-mtls-host-bridge', 'production_enabled': False, 'real_model_calls': 0,
              'host': {'system': platform.system(), 'machine': platform.machine()},
              'binary_sha256': hashlib.sha256(args.binary.read_bytes()).hexdigest(), 'checks': {}}
    with tempfile.TemporaryDirectory(prefix='xcli-auth-host-bridge-') as temporary:
        root = Path(temporary)
        root.chmod(0o700)
        fixture = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Fixture)
        fixture.calls, fixture.calls_lock = Counter(), threading.Lock()
        threading.Thread(target=fixture.serve_forever, daemon=True).start()
        state = root / 'gateway-state'
        config_path = root / 'gateway.json'
        private_write(config_path, {'version': 1, 'server': {'host': '127.0.0.1', 'port': 0, 'timeout_seconds': 10},
            'backends': {'fixture': {'type': 'openai-compatible',
                'endpoint': f'http://127.0.0.1:{fixture.server_port}/v1', 'inference_location': 'local'}},
            'models': {'xcli/text-default': {'backend': 'fixture', 'upstream_model': 'fixture-text', 'kind': 'text',
                'capabilities': ['text.generate', 'tool_calls', 'json_schema'], 'verification': 'probe_verified',
                'verified_at': '2026-10-26T00:00:00Z', 'evidence': 'synthetic-bridge-only'}}})
        def cli(*arguments):
            return execute([args.binary, '--config', config_path, 'model', '--state-dir', state, *arguments], timeout=20)
        started = False
        try:
            token = root / 'application.token'
            cli('client', 'issue', 'bridge-probe', '--token-file', str(token), '--model', 'xcli/text-default')
            status = json.loads(cli('start'))
            started = True
            host_tls = root / 'host-tls'
            fingerprint = credentials(host_tls)
            relay_settings = relay_config(host_tls, 'http://' + status['address'] + '/v1', fingerprint)
            relay_settings['timeout_seconds'] = 15
            with running_relay(host_tls, relay_settings) as (relay, native_endpoint):
                app_files = {p.relative_to(source).as_posix(): hashlib.sha256(p.read_bytes()).hexdigest()
                             for p in (source/'app').rglob('*') if p.is_file() and '__pycache__' not in p.parts}
                report['source'] = {'files': len(app_files), 'sha256': hashlib.sha256(
                    json.dumps(app_files, sort_keys=True, separators=(',', ':')).encode()).hexdigest(),
                    'internal_imports_complete': True}
                def stage(name, endpoint, client='client'):
                    destination = root / name
                    destination.mkdir(mode=0o700)
                    shutil.copytree(source/'app', destination/'app', ignore=shutil.ignore_patterns('__pycache__', '*.pyc'))
                    shutil.copy2(PROBE, destination/'probe.py')
                    shutil.copy2(token, destination/'application.token')
                    (destination/'application.token').chmod(0o600)
                    tls = destination/'tls'
                    tls.mkdir(mode=0o700)
                    for origin, target in [('server.pem', 'server.pem'), (client+'.pem', 'client.pem'), (client+'.key', 'client.key')]:
                        shutil.copy2(host_tls/origin, tls/target)
                        (tls/target).chmod(0o600)
                    private_write(tls/'client.json', {'version': 1, 'endpoint': endpoint, 'instance_id': INSTANCE,
                        'server_sha256': certificate_fingerprint((tls/'server.pem').read_bytes()),
                        'server_certificate': 'server.pem', 'client_certificate': 'client.pem', 'client_key': 'client.key'})
                    return destination
                native = stage('native', native_endpoint)
                env = {key: value for key, value in os.environ.items() if not key.startswith(('CONTRIBOS_', 'XCLI_'))
                       and key.upper() not in {'HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY'}}
                env.update(PYTHONPATH=str(native), PYTHONDONTWRITEBYTECODE='1',
                    CONTRIBOS_XCLI_BRIDGE_ENDPOINT=native_endpoint, CONTRIBOS_XCLI_BRIDGE_INSTANCE=INSTANCE,
                    CONTRIBOS_XCLI_BRIDGE_SERVER_SHA256=relay.fingerprint,
                    CONTRIBOS_XCLI_BRIDGE_CONFIG=str(native/'tls/client.json'), XCLI_BRIDGE_TOKEN_FILE=str(native/'application.token'))
                result = execute([sys.executable, '-B', native/'probe.py'], cwd=native, environment=env, timeout=45)
                report['checks']['native'] = json.loads(result)
                print('Native mTLS bridge acceptance passed', flush=True)
                if args.docker_image:
                    if not args.docker_image.startswith('sha256:') or len(args.docker_image) != 71:
                        raise ValueError('Use a pre-existing immutable local image ID')
                    endpoint = f'https://host.docker.internal:{relay.server_port}/v1'
                    def container_probe(name, client='client', rejected=False):
                        staged = stage(name, endpoint, client)
                        arguments = ['docker', 'run', '--rm', '--pull=never', '--read-only', '--network=bridge',
                            '--cap-drop=ALL', '--security-opt=no-new-privileges', '--pids-limit=64', '--memory=256m', '--cpus=1',
                            '--user', f'{os.getuid()}:{os.getgid()}', '--tmpfs', '/tmp:rw,nosuid,nodev,noexec,size=16m',
                            '--mount', f'type=bind,src={staged},dst=/probe,readonly', '--workdir=/probe',
                            '--env=PYTHONPATH=/probe', '--env=PYTHONDONTWRITEBYTECODE=1',
                            '--env=CONTRIBOS_XCLI_BRIDGE_ENDPOINT='+endpoint, '--env=CONTRIBOS_XCLI_BRIDGE_INSTANCE='+INSTANCE,
                            '--env=CONTRIBOS_XCLI_BRIDGE_SERVER_SHA256='+relay.fingerprint,
                            '--env=CONTRIBOS_XCLI_BRIDGE_CONFIG=/probe/tls/client.json', '--env=XCLI_BRIDGE_TOKEN_FILE=/probe/application.token']
                        if rejected:
                            arguments += ['--env=XCLI_BRIDGE_REJECT_ONLY=1']
                        arguments += ['--entrypoint=python', args.docker_image, '-B', '/probe/probe.py']
                        return json.loads(execute(arguments, timeout=60))
                    report['docker_image'] = args.docker_image
                    report['docker_environment'] = execute(['docker', 'info', '--format', '{{.OSType}} {{.Architecture}} {{.OperatingSystem}}'], timeout=15).strip()
                    report['checks']['docker'] = container_probe('docker-valid')
                    before = sum(fixture.calls.values())
                    report['checks']['docker_unlisted_client'] = container_probe('docker-invalid', 'unlisted-client', True)
                    assert sum(fixture.calls.values()) == before
                    print('Docker -> host loopback mTLS acceptance passed', flush=True)
                expected = 4 if args.docker_image else 2
                assert sum(fixture.calls.values()) == expected
                report['synthetic_inference_calls'] = expected
                report['checks']['no_unapproved_calls'] = True
                audit = cli('logs')
                assert audit.strip(), 'Audit evidence is missing'
                report['checks']['no_audit_token_leak'] = token.read_text().strip() not in audit
                assert report['checks']['no_audit_token_leak']
        finally:
            if started:
                cli('stop')
            fixture.shutdown()
            fixture.server_close()
    report['limitations'] = ['No production certificates installed or bridge activated',
        'No real model calls', 'Linux amd64 physical Docker and other OS acceptance remain unverified']
    args.output.write_text(json.dumps(report, indent=2)+'\n')
    print(json.dumps({'report': str(args.output), 'checks': list(report['checks']), 'real_model_calls': 0}))


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source', type=Path, required=True)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--docker-image')
    parser.add_argument('--output', type=Path, required=True)
    run(parser.parse_args())
