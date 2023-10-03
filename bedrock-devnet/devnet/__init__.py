import argparse
import logging
import os
import subprocess
import json
import socket
import calendar
import datetime
import time
import shutil
import http.client
from multiprocessing import Process, Queue

import devnet.log_setup

pjoin = os.path.join

parser = argparse.ArgumentParser(description='Bedrock devnet launcher')
parser.add_argument('--monorepo-dir', help='Directory of the monorepo', default=os.getcwd())
parser.add_argument('--allocs', help='Only create the allocs and exit', type=bool, action=argparse.BooleanOptionalAction)
parser.add_argument('--test', help='Tests the deployment, must already be deployed', type=bool, action=argparse.BooleanOptionalAction)

log = logging.getLogger()

class Bunch:
    def __init__(self, **kwds):
        self.__dict__.update(kwds)

class ChildProcess:
    def __init__(self, func, *args):
        self.errq = Queue()
        self.process = Process(target=self._func, args=(func, args))

    def _func(self, func, args):
        try:
            func(*args)
        except Exception as e:
            self.errq.put(str(e))

    def start(self):
        self.process.start()

    def join(self):
        self.process.join()

    def get_error(self):
        return self.errq.get() if not self.errq.empty() else None


def main():
    args = parser.parse_args()

    monorepo_dir = os.path.abspath(args.monorepo_dir)
    devnet_dir = pjoin(monorepo_dir, '.devnet')
    contracts_bedrock_dir = pjoin(monorepo_dir, 'packages', 'contracts-bedrock')
    deployment_dir = pjoin(contracts_bedrock_dir, 'deployments', 'devnetL1')
    op_node_dir = pjoin(args.monorepo_dir, 'op-node')
    ops_bedrock_dir = pjoin(monorepo_dir, 'ops-bedrock')
    deploy_config_dir = pjoin(contracts_bedrock_dir, 'deploy-config')
    devnet_config_path = pjoin(deploy_config_dir, 'devnetL1.json')
    devnet_config_template_path = pjoin(deploy_config_dir, 'devnetL1-template.json')
    ops_chain_ops = pjoin(monorepo_dir, 'op-chain-ops')
    sdk_dir = pjoin(monorepo_dir, 'packages', 'sdk')

    paths = Bunch(
      mono_repo_dir=monorepo_dir,
      devnet_dir=devnet_dir,
      contracts_bedrock_dir=contracts_bedrock_dir,
      deployment_dir=deployment_dir,
      l1_deployments_path=pjoin(deployment_dir, '.deploy'),
      deploy_config_dir=deploy_config_dir,
      devnet_config_path=devnet_config_path,
      devnet_config_template_path=devnet_config_template_path,
      op_node_dir=op_node_dir,
      ops_bedrock_dir=ops_bedrock_dir,
      ops_chain_ops=ops_chain_ops,
      sdk_dir=sdk_dir,
      genesis_l1_path=pjoin(devnet_dir, 'genesis-l1.json'),
      genesis_l2_path=pjoin(devnet_dir, 'genesis-l2.json'),
      allocs_path=pjoin(devnet_dir, 'allocs-l1.json'),
      addresses_json_path=pjoin(devnet_dir, 'addresses.json'),
      sdk_addresses_json_path=pjoin(devnet_dir, 'sdk-addresses.json'),
      rollup_config_path=pjoin(devnet_dir, 'rollup.json')
    )

    if args.test:
      log.info('Testing deployed devnet')
      devnet_test(paths)
      return

    os.makedirs(devnet_dir, exist_ok=True)

    if args.allocs:
        devnet_l1_genesis(paths)
        return

    log.info('Building docker images')
    run_command(['docker', 'compose', 'build', '--progress', 'plain'], cwd=paths.ops_bedrock_dir, env={
        'PWD': paths.ops_bedrock_dir
    })

    log.info('Devnet starting')
    devnet_deploy(paths)


def deploy_contracts(paths):
    wait_up(8545)
    wait_for_rpc_server('127.0.0.1:8545')
    res = eth_accounts('127.0.0.1:8545')

    response = json.loads(res)
    account = response['result'][0]
    log.info(f'Deploying with {account}')

    # send some ether to the create2 deployer account
    run_command([
        'cast', 'send', '--from', account,
        '--rpc-url', 'http://127.0.0.1:8545',
        '--unlocked', '--value', '1ether', '0x3fAB184622Dc19b6109349B94811493BF2a45362'
    ], env={}, cwd=paths.contracts_bedrock_dir)

    # deploy the create2 deployer
    run_command([
      'cast', 'publish', '--rpc-url', 'http://127.0.0.1:8545',
      '0xf8a58085174876e800830186a08080b853604580600e600039806000f350fe7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe03601600081602082378035828234f58015156039578182fd5b8082525050506014600cf31ba02222222222222222222222222222222222222222222222222222222222222222a02222222222222222222222222222222222222222222222222222222222222222'
    ], env={}, cwd=paths.contracts_bedrock_dir)

    fqn = 'scripts/Deploy.s.sol:Deploy'
    run_command([
        'forge', 'script', fqn, '--sender', account,
        '--rpc-url', 'http://127.0.0.1:8545', '--broadcast',
        '--unlocked'
    ], env={}, cwd=paths.contracts_bedrock_dir)

    shutil.copy(paths.l1_deployments_path, paths.addresses_json_path)

    log.info('Syncing contracts.')
    run_command([
        'forge', 'script', fqn, '--sig', 'sync()',
        '--rpc-url', 'http://127.0.0.1:8545'
    ], env={}, cwd=paths.contracts_bedrock_dir)

def init_devnet_l1_deploy_config(paths, update_timestamp=False):
    deploy_config = read_json(paths.devnet_config_template_path)
    if update_timestamp:
        deploy_config['l1GenesisBlockTimestamp'] = '{:#x}'.format(int(time.time()))
    write_json(paths.devnet_config_path, deploy_config)

def devnet_l1_genesis(paths):
    log.info('Generating L1 genesis state')
    init_devnet_l1_deploy_config(paths)

    geth = subprocess.Popen([
        'geth', '--dev', '--http', '--http.api', 'eth,debug',
        '--verbosity', '4', '--gcmode', 'archive', '--dev.gaslimit', '30000000',
        '--rpc.allow-unprotected-txs'
    ])

    try:
        forge = ChildProcess(deploy_contracts, paths)
        forge.start()
        forge.join()
        err = forge.get_error()
        if err:
            raise Exception(f"Exception occurred in child process: {err}")

        res = debug_dumpBlock('127.0.0.1:8545')
        response = json.loads(res)
        allocs = response['result']

        write_json(paths.allocs_path, allocs)
    finally:
        geth.terminate()


# Bring up the devnet where the contracts are deployed to L1
def devnet_deploy(paths):
    if os.path.exists(paths.genesis_l1_path):
        log.info('L1 genesis already generated.')
    else:
        log.info('Generating L1 genesis.')
        if os.path.exists(paths.allocs_path) == False:
            devnet_l1_genesis(paths)

        # It's odd that we want to regenerate the devnetL1.json file with
        # an updated timestamp different than the one used in the devnet_l1_genesis
        # function.  But, without it, CI flakes on this test rather consistently.
        # If someone reads this comment and understands why this is being done, please
        # update this comment to explain.
        init_devnet_l1_deploy_config(paths, update_timestamp=True)
        outfile_l1 = pjoin(paths.devnet_dir, 'genesis-l1.json')
        run_command([
            'go', 'run', 'cmd/main.go', 'genesis', 'l1',
            '--deploy-config', paths.devnet_config_path,
            '--l1-allocs', paths.allocs_path,
            '--l1-deployments', paths.addresses_json_path,
            '--outfile.l1', outfile_l1,
        ], cwd=paths.op_node_dir)

    log.info('Starting L1.')
    run_command(['docker', 'compose', 'up', '-d', 'l1'], cwd=paths.ops_bedrock_dir, env={
        'PWD': paths.ops_bedrock_dir
    })
    wait_up(8545)
    wait_for_rpc_server('127.0.0.1:8545')

    if os.path.exists(paths.genesis_l2_path):
        log.info('L2 genesis and rollup configs already generated.')
    else:
        log.info('Generating L2 genesis and rollup configs.')
        run_command([
            'go', 'run', 'cmd/main.go', 'genesis', 'l2',
            '--l1-rpc', 'http://localhost:8545',
            '--deploy-config', paths.devnet_config_path,
            '--deployment-dir', paths.deployment_dir,
            '--outfile.l2', pjoin(paths.devnet_dir, 'genesis-l2.json'),
            '--outfile.rollup', pjoin(paths.devnet_dir, 'rollup.json')
        ], cwd=paths.op_node_dir)

    rollup_config = read_json(paths.rollup_config_path)
    addresses = read_json(paths.addresses_json_path)

    log.info('Bringing up L2.')
    run_command(['docker', 'compose', 'up', '-d', 'l2'], cwd=paths.ops_bedrock_dir, env={
        'PWD': paths.ops_bedrock_dir
    })
    wait_up(9545)
    wait_for_rpc_server('127.0.0.1:9545')

    l2_output_oracle = addresses['L2OutputOracleProxy']
    log.info(f'Using L2OutputOracle {l2_output_oracle}')
    batch_inbox_address = rollup_config['batch_inbox_address']
    log.info(f'Using batch inbox {batch_inbox_address}')

    log.info('Bringing up DA')
    run_command(['docker', 'compose', 'up', '-d', 'da'], cwd=paths.ops_bedrock_dir, env={
        'PWD': paths.ops_bedrock_dir,
    })
    log.info('Bringing up `op-node`, `op-proposer` and `op-batcher`.')
    result = run_command(["docker", "exec", "ops-bedrock-da-1", "celestia", "bridge", "auth", "admin", "--node.store", "/home/celestia/bridge"],
        cwd=paths.ops_bedrock_dir, capture_output=True,
    )
    auth_token = result.stdout
    run_command(["docker", "compose", "up", "-d", "op-node", "op-proposer", "op-batcher"], cwd=paths.ops_bedrock_dir,
        env={
            "PWD": paths.ops_bedrock_dir,
            "L2OO_ADDRESS": l2_output_oracle,
            "SEQUENCER_BATCH_INBOX_ADDRESS": batch_inbox_address,
            "CELESTIA_NODE_AUTH_TOKEN": auth_token,
        },
    )


    log.info('Devnet ready.')


def eth_accounts(url):
    log.info(f'Fetch eth_accounts {url}')
    conn = http.client.HTTPConnection(url)
    headers = {'Content-type': 'application/json'}
    body = '{"id":2, "jsonrpc":"2.0", "method": "eth_accounts", "params":[]}'
    conn.request('POST', '/', body, headers)
    response = conn.getresponse()
    data = response.read().decode()
    conn.close()
    return data


def debug_dumpBlock(url):
    log.info(f'Fetch debug_dumpBlock {url}')
    conn = http.client.HTTPConnection(url)
    headers = {'Content-type': 'application/json'}
    body = '{"id":3, "jsonrpc":"2.0", "method": "debug_dumpBlock", "params":["latest"]}'
    conn.request('POST', '/', body, headers)
    response = conn.getresponse()
    data = response.read().decode()
    conn.close()
    return data


def wait_for_rpc_server(url):
    log.info(f'Waiting for RPC server at {url}')

    conn = http.client.HTTPConnection(url)
    headers = {'Content-type': 'application/json'}
    body = '{"id":1, "jsonrpc":"2.0", "method": "eth_chainId", "params":[]}'

    while True:
        try:
            conn.request('POST', '/', body, headers)
            response = conn.getresponse()
            conn.close()
            if response.status < 300:
                log.info(f'RPC server at {url} ready')
                return
        except Exception as e:
            log.info(f'Waiting for RPC server at {url}')
            time.sleep(1)

def devnet_test(paths):
    # Check the L2 config
    run_command(
        ['go', 'run', 'cmd/check-l2/main.go', '--l2-rpc-url', 'http://localhost:9545', '--l1-rpc-url', 'http://localhost:8545'],
        cwd=paths.ops_chain_ops,
    )

    run_command(
         ['npx', 'hardhat',  'deposit-erc20', '--network',  'devnetL1', '--l1-contracts-json-path', paths.addresses_json_path],
         cwd=paths.sdk_dir,
         timeout=8*60,
    )

    run_command(
         ['npx', 'hardhat',  'deposit-eth', '--network',  'devnetL1', '--l1-contracts-json-path', paths.addresses_json_path],
         cwd=paths.sdk_dir,
         timeout=8*60,
    )

def run_command(args, check=True, shell=False, cwd=None, env=None, timeout=None, capture_output=False):
    env = env if env else {}
    return subprocess.run(
        args,
        capture_output=capture_output,
        check=check,
        shell=shell,
        env={
            **os.environ,
            **env
        },
        cwd=cwd,
        timeout=timeout
    )


def wait_up(port, retries=10, wait_secs=1):
    for i in range(0, retries):
        log.info(f'Trying 127.0.0.1:{port}')
        s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        try:
            s.connect(('127.0.0.1', int(port)))
            s.shutdown(2)
            log.info(f'Connected 127.0.0.1:{port}')
            return True
        except Exception:
            time.sleep(wait_secs)

    raise Exception(f'Timed out waiting for port {port}.')


def write_json(path, data):
    with open(path, 'w+') as f:
        json.dump(data, f, indent='  ')


def read_json(path):
    with open(path, 'r') as f:
        return json.load(f)
