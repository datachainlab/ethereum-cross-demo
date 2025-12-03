# anvil-cross-demo

## Requirements

- [Go](https://go.dev/) 1.24
- [Docker](https://www.docker.com/products/docker-desktop)
- [jq](https://stedolan.github.io/jq/)
- Node.js v25

## Testing the demo

```
# 1) Install dependencies
npm install

# 2) Change directory
cd demo

# 3) Build artifacts and tools
make build

# 4) Start local networks (Anvil) & Deploy contracts
make network

# 5) Run demo scenarios
make demo
```

## Tear Down / Clean

```
# Stop networks and clean up
make network-down
```
