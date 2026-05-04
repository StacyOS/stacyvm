# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
forge build              # compile contracts
forge build --sizes      # compile with contract size report
forge test               # run all tests
forge test -vvv          # run tests with verbose output (used in CI)
forge test --match-test test_Increment   # run a single test by name
forge test --match-contract CounterTest  # run all tests in a contract
forge fmt                # format Solidity source files
forge fmt --check        # check formatting without writing (used in CI)
forge snapshot           # generate gas snapshots
```

Deploy a script to a network:
```bash
forge script script/Counter.s.sol:CounterScript --rpc-url <rpc_url> --private-key <key> --broadcast
```

Local node for testing:
```bash
anvil
```

## Architecture

This is a [Foundry](https://book.getfoundry.sh/) Solidity project living inside the larger `alex-the-agent` orchestrator at `orchestrator/images/evm/contracts/`. The `out/` directory holds compiled artifacts; `lib/forge-std` is a git submodule providing the test/script base contracts.

**Source layout:**
- `src/` — production contracts
- `test/` — Forge tests; each test contract extends `forge-std/Test.sol`, with `setUp()` run before every test
- `script/` — deployment scripts extending `forge-std/Script.sol`; wrap deployment calls in `vm.startBroadcast()` / `vm.stopBroadcast()`

**ABI export:** `Counter.json` at the repo root is a hand-maintained ABI file consumed by the orchestrator. When adding or changing public/external functions on a contract, update this file to match — it is tracked in git separately from the `out/` artifacts.

**Testing conventions:** regular tests are prefixed `test_`, fuzz tests are prefixed `testFuzz_` (Forge runs fuzz tests automatically when the test function has unbound parameters).

**CI** (`.github/workflows/test.yml`) runs `forge fmt --check`, `forge build --sizes`, then `forge test -vvv` on every push and pull request.
