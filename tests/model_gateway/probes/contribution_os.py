"""Real native xcli transport plus the original scoped StudioGateway boundary."""
import asyncio
import json
import os
from pathlib import Path

import httpx

from app.config import Settings
from app.model_settings import ConnectionInput, ProfileInput
from app.providers.codex_cli import CodexExecInvocation
from app.providers.contracts import ProviderStage
from app.providers.gateway import GatewayTaskAuthorizer, GatewayTaskTokenCodec, InternalModelGateway
from app.providers.gateway_http import create_model_gateway_app
from app.providers.studio import StudioGateway, runner_for_profile


async def run():
    endpoint = os.environ['XCLI_NATIVE_BASE_URL']
    os.environ['CONTRIBOS_XCLI_LOCAL_ENDPOINT'] = endpoint
    token = Path(os.environ['XCLI_NATIVE_TOKEN_FILE']).read_text().strip()
    connection = ConnectionInput(name='Synthetic native xcli', provider='xcli_local', base_url=endpoint,
                                 credential_ref='cred-' + '1' * 32).model_dump()
    profile = ProfileInput(name='Synthetic native profile', connection_id='native-connection',
                           model='xcli/text-default', structured_output='json_schema').model_dump()
    profile.update(id='native-profile', record_hash='a' * 64, connection=connection)
    class Store:
        def read(self, reference):
            assert reference == connection['credential_ref']
            return token
    class Legacy:
        async def create_response(self, payload):
            raise AssertionError('Unexpected fallback to legacy provider')
    key = b'native-offline-signing-fixture-key'
    legacy = InternalModelGateway(authorizer=GatewayTaskAuthorizer(GatewayTaskTokenCodec(key)),
                                  upstream=Legacy(), provider_name='xcli_local')
    application = create_model_gateway_app(StudioGateway(legacy, Store()))
    async with httpx.AsyncClient(transport=httpx.ASGITransport(app=application),
                                 base_url='http://contribos-model-gateway:8001') as client:
        settings = Settings(model_gateway_signing_key=key)
        runner = runner_for_profile(settings, profile, client=client)
        invocation = CodexExecInvocation(stage=ProviderStage.PLANNING, request_id='native-request',
            correlation_id='native-correlation', snapshot_id='native-snapshot', input_hash='b' * 64,
            prompt='Synthetic integration input', output_schema={
                'type': 'object', 'properties': {'ok': {'type': 'boolean'}}, 'required': ['ok'],
            }, model=profile['model'])
        result = await runner.complete(invocation)
        assert json.loads(result.content) == {'ok': True}
        credential = runner.broker.issue(invocation)
        response = await client.post('/v1/chat/completions', headers={'Authorization': 'Bearer ' + credential.token},
            json={'model': profile['model'], '_contribos_profile': {
                **profile, 'connection': {**connection, 'base_url': 'http://127.0.0.1:1/v1'},
            }})
        assert response.status_code == 403
    return {'native_client': 'contribution-os', 'frozen_profile': True, 'scoped_task_token': True,
            'json_schema': True, 'tampered_profile_rejected': True, 'container_bridge': 'unsupported'}


if __name__ == '__main__':
    print(json.dumps(asyncio.run(run())))
