# EVM Workspace

This workspace contains two sub-projects:

```
/workspace/
  contracts/   # Foundry/Solidity smart contracts
  frontend/    # Next.js 16 frontend (bun)
```

## Starting the dev server

The Next.js dev server runs from the `frontend` directory on port 3000:

```bash
cd /workspace/frontend && bun dev
```

Always start the dev server from `/workspace/frontend`, not from `/workspace`.

## Sub-project docs

- Frontend: see `frontend/CLAUDE.md` or `frontend/AGENTS.md`
- Contracts: see `contracts/CLAUDE.md` or `contracts/AGENTS.md`
