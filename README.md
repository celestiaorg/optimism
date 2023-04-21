<div align="center">
  <br />
  <br />
  <a href="https://optimism.io"><img alt="Optimism" src="https://raw.githubusercontent.com/ethereum-optimism/brand-kit/main/assets/svg/OPTIMISM-R.svg" width=600></a>
  <h3><a href="https://optimism.io">Optimism</a> is a low-cost and lightning-fast Ethereum L2 blockchain, built with the OP Stack.</h3>
  <br />
  <h3>+</h3>
  <a href="https://celestia.org"><img alt="Celestia" src="docs/op-stack/src/assets/docs/understand/Celestia-logo-color-color.svg" width=600></a>
  <h3><a href="https://celestia.org">Celestia</a> is a modular data availability network that securely scales with the number of users, making it easy for anyone to launch their own blockchain.</h3>
  <br />
</div>

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**

- [What is Optimism?](#what-is-optimism)
- [Documentation](#documentation)
- [Specification](#specification)
- [Community](#community)
- [Contributing](#contributing)
- [Security Policy and Vulnerability Reporting](#security-policy-and-vulnerability-reporting)
- [Directory Structure](#directory-structure)
- [Development and Release Process](#development-and-release-process)
  - [Overview](#overview)
  - [Production Releases](#production-releases)
  - [Development branch](#development-branch)
- [License](#license)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## What is Optimism?

[Optimism](https://www.optimism.io/) is a project dedicated to scaling Ethereum's technology and expanding its ability to coordinate people from across the world to build effective decentralized economies and governance systems. The [Optimism Collective](https://www.optimism.io/vision) builds open-source software that powers scalable blockchains and aims to address key governance and economic challenges in the wider Ethereum ecosystem. Optimism operates on the principle of **impact=profit**, the idea that individuals who positively impact the Collective should be proportionally rewarded with profit. **Change the incentives and you change the world.**
If you're looking to run the OP Stack + Celestia setup for this repository, please visit the [Optimism & Celestia guides and tutorials](https://docs.celestia.org/developers/intro-to-op-stack/) to get started.

In this repository you'll find numerous core components of the OP Stack, the decentralized software stack maintained by the Optimism Collective that powers Optimism and forms the backbone of blockchains like [OP Mainnet](https://explorer.optimism.io/) and [Base](https://base.org). The OP Stack is designed to be aggressively open-source — you are welcome to explore, modify, and extend this code.
The OP Stack powers Optimism, an Ethereum L2 blockchain, and forms the technical foundation for the [the Optimism Collective](https://app.optimism.io/announcement)—a group committed to the **impact=profit** principle. This principle rewards individuals for their positive contributions to the collective.

Optimism addresses critical coordination failures in the crypto ecosystem, such as funding public goods and infrastructure. The OP Stack focuses on creating a shared, open-source system for developing new L2 blockchains within the proposed Superchain ecosystem, promoting collaboration and preventing redundant efforts.

As Optimism evolves, the OP Stack will adapt, encompassing components ranging from blockchain infrastructure to governance systems. This software suite aims to simplify L2 blockchain creation while supporting the growth and development of the Optimism ecosystem.

## What is Celestia?

Celestia is a modular consensus and data network, built to enable anyone to easily deploy their own blockchain with minimal overhead.

Celestia is a minimal blockchain that only orders and publishes transactions and does not execute them. By decoupling the consensus and application execution layers, Celestia modularizes the blockchain technology stack and unlocks new possibilities for decentralized application builders. Lean more at [Celestia.org](https://celestia.org).

## Maintenance

The maintenance guide for this repository can be found in the Wiki tab of the repository or [here](https://github.com/celestiaorg/optimism/wiki).

## Documentation

- If you want to build on top of OP Mainnet, refer to the [Optimism Documentation](https://docs.optimism.io)
- If you want to build your own OP Stack based blockchain, refer to the [OP Stack Guide](https://docs.optimism.io/stack/getting-started) and make sure to understand this repository's [Development and Release Process](#development-and-release-process)

## Specification

Detailed specifications for the OP Stack can be found within the [OP Stack Specs](https://github.com/ethereum-optimism/specs) repository.
If you're interested in the technical details of how Optimism works, refer to the [Optimism Protocol Specification](https://github.com/ethereum-optimism/specs).
If you want to build on top of Celestia, take a look at the documentation at [docs.celestia.org](https://docs.celestia.org).

If you want to learn more about the OP Stack, check out the documentation at [stack.optimism.io](https://stack.optimism.io/).

## Community

### Optimism

General discussion happens most frequently on the [Optimism discord](https://discord.gg/optimism).
Governance discussion can also be found on the [Optimism Governance Forum](https://gov.optimism.io/).

### Celestia

General discussion happens most frequently on the [Celestia discord](https://discord.com/invite/YsnTPcSfWQ).
Other discussions can be found on the [Celestia forum](https://forum.celestia.org).

## Community

General discussion happens most frequently on the [Optimism discord](https://discord.gg/optimism).
Governance discussion can also be found on the [Optimism Governance Forum](https://gov.optimism.io/).

## Contributing

The OP Stack is a collaborative project. By collaborating on free, open software and shared standards, the Optimism Collective aims to prevent siloed software development and rapidly accelerate the development of the Ethereum ecosystem. Come contribute, build the future, and redefine power, together.

[CONTRIBUTING.md](./CONTRIBUTING.md) contains a detailed explanation of the contributing process for this repository. Make sure to use the [Developer Quick Start](./CONTRIBUTING.md#development-quick-start) to properly set up your development environment.

[Good First Issues](https://github.com/ethereum-optimism/optimism/issues?q=is:open+is:issue+label:D-good-first-issue) are a great place to look for tasks to tackle if you're not sure where to start, and see [CONTRIBUTING.md](./CONTRIBUTING.md) for info on larger projects.

## e2e testing

Please refer to the canonical [Security Policy](https://github.com/ethereum-optimism/.github/blob/master/SECURITY.md) document for detailed information about how to report vulnerabilities in this codebase.
Bounty hunters are encouraged to check out the [Optimism Immunefi bug bounty program](https://immunefi.com/bounty/optimism/).
The Optimism Immunefi program offers up to $2,000,042 for in-scope critical vulnerabilities.
This repository has updated end-to-end tests in the `op-e2e` package to work with
Celestia as the data availability (DA) layer.

Currently, the tests assume a working [Celestia devnet](https://github.com/rollkit/local-celestia-devnet) running locally:

```bash
docker run -p 26650:26650 ghcr.io/rollkit/local-celestia-devnet:v0.12.7
```

The e2e tests can be triggered with:

```bash
cd $HOME/optimism
cd op-e2e
OP_E2E_DISABLE_PARALLEL=true OP_E2E_CANNON_ENABLED=false OP_NODE_DA_RPC=localhost:26650 OP_BATCHER_DA_RPC=localhost:26650 make test
```

## Bridging

If you have the OP Stack + Celestia setup running, you can test out bridging from the L1
to the L2.

To do this, first navigate to the `packages/contracts-bedrock` directory and create a
`.env` file with the following contents:

```bash
L1_PROVIDER_URL=http://localhost:8545
L2_PROVIDER_URL=http://localhost:9545
PRIVATE_KEY=bf7604d9d3a1c7748642b1b7b05c2bd219c9faa91458b370f85e5a40f3b03af7
```

Then, run the following from the same directory:

```bash
npx hardhat deposit --network devnetL1 --l1-provider-url http://localhost:8545 --l2-provider-url http://localhost:9545 --amount-eth <AMOUNT> --to <ADDRESS>
```

## Directory Structure

<pre>
├── <a href="./docs">docs</a>: A collection of documents including audits and post-mortems
├── <a href="./kurtosis-devnet">kurtosis-devnet</a>: OP-Stack Kurtosis devnet
├── <a href="./op-batcher">op-batcher</a>: L2-Batch Submitter, submits bundles of batches to L1
├── <a href="./op-chain-ops">op-chain-ops</a>: State surgery utilities
├── <a href="./op-challenger">op-challenger</a>: Dispute game challenge agent
├── <a href="./op-e2e">op-e2e</a>: End-to-End testing of all bedrock components in Go
├── <a href="./op-node">op-node</a>: rollup consensus-layer client
├── <a href="./op-preimage">op-preimage</a>: Go bindings for Preimage Oracle
├── <a href="./op-program">op-program</a>: Fault proof program
├── <a href="./op-proposer">op-proposer</a>: L2-Output Submitter, submits proposals to L1
├── <a href="./op-service">op-service</a>: Common codebase utilities
├── <a href="./op-ufm">op-ufm</a>: Simulations for monitoring end-to-end transaction latency
├── <a href="./op-wheel">op-wheel</a>: Database utilities
├── <a href="./ops">ops</a>: Various operational packages
├── <a href="./packages">packages</a>
│   ├── <a href="./packages/contracts-bedrock">contracts-bedrock</a>: OP Stack smart contracts
├── <a href="./semgrep">semgrep</a>: Semgrep rules and tests
</pre>

## Development and Release Process

### Overview

Please read this section carefully if you're planning to fork or make frequent PRs into this repository.

### Production Releases

Production releases are always tags, versioned as `<component-name>/v<semver>`.
For example, an `op-node` release might be versioned as `op-node/v1.1.2`, and  smart contract releases might be versioned as `op-contracts/v1.0.0`.
Release candidates are versioned in the format `op-node/v1.1.2-rc.1`.
We always start with `rc.1` rather than `rc`.

For contract releases, refer to the GitHub release notes for a given release which will list the specific contracts being released. Not all contracts are considered production ready within a release and many are under active development.

Tags of the form `v<semver>`, such as `v1.1.4`, indicate releases of all Go code only, and **DO NOT** include smart contracts.
This naming scheme is required by Golang.
In the above list, this means these `v<semver>` releases contain all `op-*` components and exclude all `contracts-*` components.

`op-geth` embeds upstream geth’s version inside its own version as follows: `vMAJOR.GETH_MAJOR GETH_MINOR GETH_PATCH.PATCH`.
Basically, geth’s version is our minor version.
For example if geth is at `v1.12.0`, the corresponding op-geth version would be `v1.101200.0`.
Note that we pad out to three characters for the geth minor version and two characters for the geth patch version.
Since we cannot left-pad with zeroes, the geth major version is not padded.

See the [Node Software Releases](https://docs.optimism.io/builders/node-operators/releases) page of the documentation for more information about releases for the latest node components.

The full set of components that have releases are:

- `op-batcher`
- `op-contracts`
- `op-challenger`
- `op-node`
- `op-proposer`

All other components and packages should be considered development components only and do not have releases.

### Development branch

The primary development branch is [`develop`](https://github.com/ethereum-optimism/optimism/tree/develop/).
`develop` contains the most up-to-date software that remains backwards compatible with the latest experimental [network deployments](https://docs.optimism.io/chain/networks).
If you're making a backwards compatible change, please direct your pull request towards `develop`.

**Changes to contracts within `packages/contracts-bedrock/src` are usually NOT considered backwards compatible.**
Some exceptions to this rule exist for cases in which we absolutely must deploy some new contract after a tag has already been fully deployed.
If you're changing or adding a contract and you're unsure about which branch to make a PR into, default to using a feature branch.
Feature branches are typically used when there are conflicts between 2 projects touching the same code, to avoid conflicts from merging both into `develop`.

## License

All other files within this repository are licensed under the [MIT License](https://github.com/ethereum-optimism/optimism/blob/master/LICENSE) unless stated otherwise.
