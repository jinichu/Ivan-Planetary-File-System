# Ivan Planetary File System

The original implementation of the Interplanetary File System (IPFS) was meant to provide an infrastructure for a distributed file system for the dissemination of versioned data. The system supports file chunk versioning using cryptographic hashes, like Git, and uses a BitTorrent-like dissemination protocol, allowing the system to work without a centralized server. Although IPFS supports node verification through public keys, data transferred between intermediate nodes from a source to a destination have unencrypted access to file contents. This allows for potentially malicious third parties to easily censor documents while enroute to the end nodes.

Thus, the Ivan Planetary File System replicates a subset of IPFS features while also extending IPFS to be able to encrypt data between the source and destination such that intermediate nodes are not able to peek at the file contents being transferred through the network. It also publishing and subscription system that allows for end users to subscribe to a reference, and listen to messages that are published to the reference on a channel. 

## Instructions

Git clone or copy the project code into a folder in your $GOPATH/ directory.

To run and install this code you'll need to install protoc (Google's Protocol
Buffer compiler).

You can then run `make deps` to install all of the go dependencies and then
`make` to build the command line client and server binaries as well as run the
tests.

To launch a node you can run `./proj2 -bind :8181`, second node should be run as
`./proj2 -bind :8282 -bootstrap localhost:8181 -path tmp/node2`.

A command line interface can be run via `./ipfs localhost:8181` to connect to the
first node. Run `help` to find out more about those commands.

There's a web interface also available at the node address (ex
https://localhost:8181) which runs on every node.

## Commands

`get <document_access_id>`
Fetches a document with this access ID and returns the contents of the document. The access ID is in the format of document_id:access_key. Since all documents are encrypted, the access key is used to decrypt the document so that the contents can be retrieved. If access_id belongs to a directory (a document with children), it will return a list of all the children documents’ names and their access IDs instead.

`add <path/to/file>`
Adds a local document to the IPFS and returns the access ID of the document, in the format of document_id:access_key. 

`add -r <path/to/directory>`
Adds a local directory to the IPFS and returns the access ID of the document, in the format of document_id:access_key. 

`add -c <documents>`
Creates a parent document to a list of existing documents in the IPFS and returns the access ID of the document. Documents must be in the format of file_name,access_id and each document must be semicolon separated. For example:

index.html,document1_id:access_key1;foo.html,document2_id:access_key2

`peers list`
Lists all of the peer addresses of this node. 

`peers add <node_address>`
Add a peer to this node using the given address.

`reference get <reference_access_id>`
Fetches what this reference points to (either a document or another reference) and returns its record, in the format of document@document_id:access_key or reference@reference_id:access_key. 

`reference add <record> <path/to/priv_key>` 
Adds a reference to the IPFS (or updates an existing reference) and returns the access ID. The record is in the format of document@document_id:access_key or reference@reference_id:access_key. The returned access ID is in the format of reference_id:access_key.

`publish <message> <path/to/priv_key>`
Publishes a message to a reference on a channel. Nodes subscribed to this reference will then see the message. 

`subscribe <reference_id>`   
Subscribes to an existing reference and listens for messages on a channel. If a message is published to this reference, they will be seen on this channel.
    
`quit`   
Disconnects from this node and exits the Ivan Planetary File System.
