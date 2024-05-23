package plasma

/*
* celestia storage
* uses celestia for storage: https://github.com/celestiaorg/celestia-openrpc
* for local testing, run a celestia devnet:
* docker run -t -i -p 26650:26650 -p 26657:26657 -p 26658:26658 -p 26659:26659 -p 9090:9090 ghcr.io/rollkit/local-celestia-devnet:latest
* wait for a few seconds, then fetch the node auth token (assuming only one container is running)
* docker exec $(docker ps -q) celestia bridge auth admin --node.store /home/celestia/bridge
* the devnet runs a celestia-node instance which listens on localhost:26658
* generate a namesapce: export NAMESPACE=00000000000000000000000000000000000000$(openssl rand -hex 10)
* these can be passed to the daserver as follows:
* ./daserver --addr 127.0.0.1 --port 3100 --celestia.server http://127.0.0.1:26658 --celestia.auth-token eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJBbGxvdyI6WyJwdWJsaWMiLCJyZWFkIiwid3JpdGUiLCJhZG1pbiJdfQ.c5MwgJYRPG-WrN8FxiOWXAp6ddGVs8W7X_AmHDFN70Q --celestia.namespace 0000000000000000000000000000000000000001020304050607080910
*/
