Special instructions for compiling/running the code should be included in this file.

Git clone or copy the project code into a folder in your $GOPATH/ directory.

To run and install this code you'll need to install protoc (Google's Protocol
Buffer compiler).

You can then run `make deps` to install all of the go dependencies and then
`make` to build the command line client and server binaries as well as run the
tests.

To launch a node you can run `./proj2 -bind :8181`, second node should be run as
`./proj2 -bind :8282 -bootstrap localhost:8181 -path tmp/node2`.

Command line interface can be run via `./ipfs localhost:8181` to connect to the
first node. Run `help` to find out more about those commands.

There's a web interface also available at the node address (ex
https://localhost:8181) which runs on every node.
