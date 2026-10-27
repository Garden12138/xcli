"""Frozen StudioGateway -> real mTLS relay -> xcli, using synthetic inputs only."""
import asyncio
from datetime import datetime, timedelta, timezone
import json
import os
from pathlib import Path

import httpx

from app.config import Settings
from app.model_settings import ConnectionInput, ProfileInput
from app.providers.codex_cli import CodexExecInvocation
from app.providers.contracts import ProviderStage, ProviderRunError
from app.providers.gateway import GatewayTaskAuthorizer, GatewayTaskTokenCodec, InternalModelGateway
from app.providers.gateway_http import create_model_gateway_app
from app.providers.studio import StudioGateway, runner_for_profile
from app.providers.xcli_bridge import INSTANCE_HEADER, XcliBridgeClient


async def run():
    endpoint = os.environ['CONTRIBOS_XCLI_BRIDGE_ENDPOINT']
    instance = os.environ['CONTRIBOS_XCLI_BRIDGE_INSTANCE']
    fingerprint = os.environ['CONTRIBOS_XCLI_BRIDGE_SERVER_SHA256']
    token = Path(os.environ['XCLI_BRIDGE_TOKEN_FILE']).read_text().strip()
    if os.environ.get('XCLI_BRIDGE_REJECT_ONLY'):
        try:
            await XcliBridgeClient(instance, fingerprint).request(endpoint, '/models', credential=token, timeout=5)
        except ProviderRunError as error:
            assert error.code == 'xcli_transport_failed'
            return {'unlisted_client_rejected': True}
        raise AssertionError('Unlisted TLS client was accepted')
    connection = ConnectionInput(name='Synthetic host bridge', provider='xcli_bridge', base_url=endpoint,
                                 credential_ref='cred-' + '1' * 32).model_dump()
    assert connection['bridge_instance'] == instance
    profile = ProfileInput(name='Synthetic bridge profile', connection_id='bridge-connection',
                           model='xcli/text-default', structured_output='json_schema').model_dump()
    profile.update(id='bridge-profile', record_hash='a' * 64, connection=connection)
    class Store:
        def read(self, reference):
            assert reference == connection['credential_ref']
            return token
    class NoFallback:
        async def create_response(self, payload):
            raise AssertionError('Unexpected provider fallback')
    key = b'bridge-offline-signing-fixture-key'
    legacy = InternalModelGateway(authorizer=GatewayTaskAuthorizer(GatewayTaskTokenCodec(key)),
                                  upstream=NoFallback(), provider_name='xcli_bridge')
    application = create_model_gateway_app(StudioGateway(legacy, Store()))
    async with httpx.AsyncClient(transport=httpx.ASGITransport(app=application),
                                 base_url='http://contribos-model-gateway:8001') as client:
        runner = runner_for_profile(Settings(model_gateway_signing_key=key), profile, client=client)
        runner.broker.max_requests = 1
        invocation = CodexExecInvocation(stage=ProviderStage.PLANNING, request_id='bridge-request',
            correlation_id='bridge-correlation', snapshot_id='bridge-snapshot', input_hash='b' * 64,
            prompt='Synthetic bridge integration input', output_schema={
                'type': 'object', 'properties': {'ok': {'type': 'boolean'}}, 'required': ['ok'],
            }, model=profile['model'])
        result = await runner.complete(invocation)
        assert json.loads(result.content) == {'ok': True}
        credential = runner.broker.issue(invocation)
        body = {'model': profile['model'], 'messages': [{'role': 'user', 'content': 'Synthetic scope probe'}],
                '_contribos_profile': profile}
        async def post(payload, value=credential.token):
            return await client.post('/v1/chat/completions', headers={'Authorization': 'Bearer '+value}, json=payload)
        for altered in [{**connection, 'base_url': 'https://127.0.0.1:1/v1'},
                        {**connection, 'bridge_instance': 'another-synthetic-instance'},
                        {**connection, 'bridge_server_sha256': '0'*64}]:
            response = await post({**body, '_contribos_profile': {**profile, 'connection': altered}})
            assert response.status_code == 403
        response = await post(body)
        assert response.status_code == 200
        response = await post(body)
        assert response.status_code == 429  # Same one-use task authorization cannot be replayed.
        expired = runner.broker.issue(invocation, now=datetime.now(timezone.utc)-timedelta(hours=1))
        assert (await post(body, expired.token)).status_code == 401
        new_credential = runner.broker.issue(invocation)
        os.environ['CONTRIBOS_XCLI_BRIDGE_INSTANCE'] = 'synthetic-rotated-instance'
        assert (await post(body, new_credential.token)).status_code == 403
        os.environ['CONTRIBOS_XCLI_BRIDGE_INSTANCE'] = instance
        os.environ['CONTRIBOS_XCLI_BRIDGE_SERVER_SHA256'] = '0'*64
        assert (await post(body, new_credential.token)).status_code == 403
        os.environ['CONTRIBOS_XCLI_BRIDGE_SERVER_SHA256'] = fingerprint
    try:
        await XcliBridgeClient(instance, fingerprint).request(endpoint, '/models', credential='synthetic-invalid-app-token', timeout=5)
    except ProviderRunError:
        pass
    else:
        raise AssertionError('Relay bypassed xcli application token authentication')
    return {'frozen_profile': True, 'mutual_tls': True, 'json_schema': True, 'instance_bound': True,
            'tampered_profile_rejected': True, 'task_token_replay_rejected': True,
            'expired_task_token_rejected': True, 'rotation_invalidates_old_profile': True, 'certificate_rotation_invalidates_old_profile': True,
            'independent_application_token_enforced': True}


if __name__ == '__main__':
    print(json.dumps(asyncio.run(run())))
