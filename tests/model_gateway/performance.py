#!/usr/bin/env python3
"""Bounded localhost latency/RSS baseline for a built gateway; no real models."""
import argparse
import base64
from collections import Counter
from concurrent.futures import ThreadPoolExecutor
import hashlib
import http.server
import json
import platform
from pathlib import Path
import random
import statistics
import struct
import subprocess
import tempfile
import threading
import time
import urllib.request
import zlib


def png_fixture():
    def chunk(kind, content):
        return struct.pack('!I', len(content)) + kind + content + struct.pack('!I', zlib.crc32(kind + content))
    rng = random.Random(42)
    raw = b''.join(b'\0' + rng.randbytes(1024 * 4) for _ in range(1024))
    return b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', struct.pack('!IIBBBBB', 1024, 1024, 8, 6, 0, 0, 0)) + chunk(b'IDAT', zlib.compress(raw, 1)) + chunk(b'IEND', b'')


class Fixture(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        request = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        with self.server.lock:
            self.server.calls[self.path] += 1
        if self.path.endswith('/images/generations'):
            assert request['model'] == 'fixture-image'
            response = {'created': 1, 'data': [{'b64_json': self.server.image}]}
        else:
            assert request['model'] == 'fixture-text'
            response = {'id': 'fixture', 'object': 'chat.completion', 'created': 1, 'choices': [
                {'index': 0, 'message': {'role': 'assistant', 'content': 'synthetic result'}, 'finish_reason': 'stop'},
            ]}
        data = json.dumps(response).encode()
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, *args):
        pass


def command(binary, config, state, *args):
    result = subprocess.run([str(binary), '--config', str(config), 'model', '--state-dir', str(state), *args],
                            capture_output=True, text=True, timeout=20)
    if result.returncode:
        raise RuntimeError('Gateway baseline CLI failed: ' + result.stderr)
    return result.stdout


def rss(pid):
    return int(subprocess.check_output(['ps', '-o', 'rss=', '-p', str(pid)], text=True).strip())


def summarize(samples):
    ordered = sorted(samples)
    return {'requests': len(samples), 'p50_ms': round(statistics.median(ordered), 3),
            'p95_ms': round(ordered[max(0, int(len(ordered) * .95 + .9999) - 1)], 3), 'max_ms': round(max(ordered), 3)}


def run(binary):
    report = {'evidence': 'localhost-fixture-performance-baseline', 'real_model_calls': 0,
              'platform': platform.system(), 'architecture': platform.machine(),
              'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
              'rss_sampling_interval_ms': 50, 'cases': {}}
    with tempfile.TemporaryDirectory(prefix='xcli-performance-') as temporary:
        directory = Path(temporary)
        upstream = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Fixture)
        upstream.lock, upstream.calls = threading.Lock(), Counter()
        image_bytes = png_fixture()
        upstream.image = base64.b64encode(image_bytes).decode()
        threading.Thread(target=upstream.serve_forever, daemon=True).start()
        config, state = directory / 'gateway.yaml', directory / 'state'
        config.write_text(json.dumps({'version': 1, 'server': {'host': '127.0.0.1', 'port': 0, 'max_concurrent_requests': 3},
            'backends': {'fixture': {'type': 'openai-compatible', 'endpoint': f'http://127.0.0.1:{upstream.server_port}/v1', 'inference_location': 'local'}},
            'models': {
                'xcli/text-default': {'backend': 'fixture', 'upstream_model': 'fixture-text', 'kind': 'text', 'capabilities': ['text.generate']},
                'xcli/image-default': {'backend': 'fixture', 'upstream_model': 'fixture-image', 'kind': 'image', 'capabilities': ['image.generate'],
                    'verification': 'probe_verified', 'verified_at': '2026-10-26T00:00:00Z', 'evidence': 'synthetic-performance-fixture-only'},
            }}))
        config.chmod(0o600)
        started = False
        stopper = threading.Event()
        sampler = None
        try:
            token_file = directory / 'app.token'
            command(binary, config, state, 'client', 'issue', 'performance-fixture', '--model', 'xcli/text-default', '--model', 'xcli/image-default',
                    '--requests-per-minute', '10000', '--token-file', str(token_file))
            token = token_file.read_text().strip()
            service = json.loads(command(binary, config, state, 'start'))
            started = True
            gateway = 'http://' + service['address'] + '/v1'
            direct = f'http://127.0.0.1:{upstream.server_port}/v1'
            observations = [rss(service['pid'])]
            report['idle_rss_kib'] = observations[0]
            def sample():
                while not stopper.wait(.05):
                    observations.append(rss(service['pid']))
            sampler = threading.Thread(target=sample, daemon=True)
            sampler.start()
            def case(name, base, model, count, concurrency, size=32, image=False):
                if image:
                    payload = {'model': model, 'prompt': 'Synthetic image baseline', 'size': '1024x1024', 'n': 1, 'response_format': 'b64_json'}
                else:
                    payload = {'model': model, 'messages': [{'role': 'user', 'content': ('synthetic ' * (size // 10 + 1))[:size]}]}
                encoded = json.dumps(payload).encode()
                def request(_):
                    headers = {'Content-Type': 'application/json'}
                    if base == gateway:
                        headers['Authorization'] = 'Bearer ' + token
                    req = urllib.request.Request(base + ('/images/generations' if image else '/chat/completions'), data=encoded, headers=headers)
                    start = time.perf_counter()
                    with urllib.request.build_opener(urllib.request.ProxyHandler({})).open(req, timeout=30) as response:
                        result = json.load(response)
                    if image:
                        assert len(result['data']) == 1 and len(result['data'][0]['b64_json']) == len(upstream.image)
                    else:
                        assert result['choices'][0]['message']['content'] == 'synthetic result'
                    return (time.perf_counter() - start) * 1000
                started_at = time.perf_counter()
                with ThreadPoolExecutor(max_workers=concurrency) as pool:
                    samples = list(pool.map(request, range(count)))
                report['cases'][name] = {**summarize(samples), 'concurrency': concurrency, 'request_bytes': len(encoded),
                                         'elapsed_seconds': round(time.perf_counter() - started_at, 3), 'sampled_rss_kib_after': rss(service['pid'])}
            case('direct_fixture_small_text', direct, 'fixture-text', 60, 1)
            case('gateway_small_text', gateway, 'xcli/text-default', 60, 1)
            case('gateway_three_clients', gateway, 'xcli/text-default', 90, 3)
            case('gateway_large_text_three_clients', gateway, 'xcli/text-default', 12, 3, size=512 * 1024)
            case('gateway_images_three_clients', gateway, 'xcli/image-default', 6, 3, image=True)
            observations.append(rss(service['pid']))
            report['sampled_peak_rss_kib'] = max(observations)
            report['rss_samples'] = len(observations)
            report['image_fixture_bytes'] = len(image_bytes)
            report['image_fixture_dimensions'] = '1024x1024'
            report['fixture_requests'] = dict(upstream.calls)
            assert token not in command(binary, config, state, 'logs')
            report['limitations'] = ['loopback fixture includes Python client/server overhead', 'sampled RSS can miss short peaks',
                                     'does not measure real model latency, suspend/resume, or other physical hosts']
        finally:
            stopper.set()
            if sampler:
                sampler.join(timeout=2)
            if started:
                command(binary, config, state, 'stop')
            upstream.shutdown()
            upstream.server_close()
    return report


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    arguments = parser.parse_args()
    result = run(arguments.binary.resolve())
    arguments.output.write_text(json.dumps(result, indent=2) + '\n')
    print(json.dumps(result))
