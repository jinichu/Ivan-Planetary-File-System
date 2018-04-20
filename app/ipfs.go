package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"log"
	"mime"
	"os"
	"path/filepath"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/config"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Not enough arguments.")
		return
	}
	// Set up RPC connection to client
	creds := credentials.NewTLS(&tls.Config{
		Rand:               rand.Reader,
		InsecureSkipVerify: true,
	})

	ctx := context.TODO()
	ctxDial, _ := context.WithTimeout(ctx, 2*time.Second)
	conn, err := grpc.DialContext(ctxDial, os.Args[1],
		grpc.WithTransportCredentials(creds),
		grpc.WithBlock(),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(int(config.GRPCMsgSize)),
			grpc.MaxCallSendMsgSize(int(config.GRPCMsgSize)),
		),
	)

	if err != nil {
		log.Fatal(err)
	}
	client := serverpb.NewClientClient(conn)
	fmt.Printf("\n 🌠  Welcome to the Ivan Planetary File System. Type 'help' to list all options. 🌠 \n\n")
	start(client, ctx)
}

func start(client serverpb.ClientClient, ctx context.Context) {
	for {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("ipfs> ")
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("%+v\n", err)
			os.Exit(0)
		}
		input = strings.TrimSpace(input)
		cmd := strings.Split(input, " ")
		switch cmd[0] {
		case "get":
			get(cmd, client, ctx)
		case "add":
			add(cmd, client, ctx)
		case "peers":
			peers(cmd, client, ctx)
		case "reference":
			reference(cmd, client, ctx)
		case "publish":
			publish(cmd, client, ctx)
		case "subscribe":
			subscribe(cmd, client, ctx)
		case "help":
			fmt.Printf("\n 🚀  List of options: \n\n")
			fmt.Println("	get <document_access_id>		   Fetch a document")
			fmt.Println("	add <path/to/file>		  	   Add a document to this node")
			fmt.Println("	add -r <path/to/dir>		  	   Add a directory to this node")
			fmt.Println("	add -c <documents>		  	   Create a parent to a list of existing documents")
			fmt.Println("	peers list				   List this node's peers")
			fmt.Println("	peers add <node_address>	  	   Add a peer to this node")
			fmt.Println("	reference get <reference_access_id>        Fetch what this reference points to")
			fmt.Println("	reference add <record> <path/to/priv_key>  Add or update a reference")
			fmt.Println("	publish <message> <path/to/priv_key>	   Publish a message on a channel")
			fmt.Println("	subscribe <reference_id>		   Listen for messages on a channel")
			fmt.Printf("	quit					   Exit the program\n\n")
		case "quit":
			fmt.Println("Exiting program... Goodbye. 🌙")
			os.Exit(1)
		default:
			fmt.Println("Invalid command. Type 'help' to list all options. 👽")
		}
	}
}

func get(cmd []string, client serverpb.ClientClient, ctx context.Context) {
	if len(cmd) != 2 {
		fmt.Println("Incorrect number of arguments. Please specify a document ID and access key.")
	} else {
		if !strings.Contains(cmd[1], ":") {
			fmt.Println("Please follow the following format: 'document_id:access_key'.")
			return
		}
		args := &serverpb.GetRequest{
			AccessId: cmd[1],
		}
		resp, err := client.Get(ctx, args)
		if err != nil {
			fmt.Println(err)
		} else {
			if resp.GetDocument().GetContentType() == "directory" {
				fmt.Println("Child documents:")
				for k, v := range resp.GetDocument().GetChildren() {
					fmt.Println(k + ": " + v)
				}
			} else {
				fmt.Printf("%s\n", resp.Document.GetData())
			}
		}
	}
}

func add(cmd []string, client serverpb.ClientClient, ctx context.Context) {
	if len(cmd) < 2 {
		fmt.Println("Incorrect number of arguments. Please specify the path to the file or directory you wish to add.")
	} else if len(cmd) == 2 && cmd[1] != "-r" && cmd[1] != "-c" {
		// Adding a single file
		data, err := ioutil.ReadFile(cmd[1])
		if err != nil {
			fmt.Println(err)
		} else {
			contentType := getContentType(cmd[1])
			args := &serverpb.AddRequest{
				Document: &serverpb.Document{
					Data:        data,
					ContentType: contentType,
				},
			}
			resp, err := client.Add(ctx, args)
			if err != nil {
				fmt.Println(err)
			} else {
				fmt.Println("Access ID: " + resp.GetAccessId())
			}
		}
	} else if cmd[1] == "-r" && len(cmd) == 3 {
		// Recursively add files (adding a directory)
		dir, err := os.Stat(cmd[2])
		if err != nil {
			fmt.Println(err)
			return
		}
		if !dir.Mode().IsDir() {
			fmt.Println("Not a directory.")
			return
		}
		hash, err := addDir(cmd[2], ctx, client)
		if err != nil {
			fmt.Println(err)
		} else {
			fmt.Println("Access ID: " + hash)
		}
	} else if cmd[1] == "-r" && len(cmd) != 3 {
		fmt.Println("Please specify the path to the directory you wish to add.")
	} else if cmd[1] == "-c" && len(cmd) == 3 {
		// Creating a parent for a list of existing documents
		if !strings.Contains(cmd[2], ",") || !strings.Contains(cmd[2], ":") {
			fmt.Println("Documents should be in the format of 'name1,document1_id:access_key1;name2,document2_id:access_key2'.")
		}
		document := &serverpb.Document{
			ContentType: "directory",
			Children:    make(map[string]string),
		}
		pairs := strings.Split(cmd[2], ";")
		for _, pair := range pairs {
			pair = strings.TrimSpace(pair)
			child := strings.Split(pair, ",")
			document.Children[child[0]] = child[1]
		}
		args := &serverpb.AddRequest{
			Document: document,
		}
		resp, err := client.Add(ctx, args)
		if err != nil {
			fmt.Println(err)
		} else {
			fmt.Println("Access ID: " + resp.GetAccessId())
		}
	} else if cmd[1] == "-c" && len(cmd) != 3 {
		fmt.Println("Please specify the list of documents you wish to create a parent for, in the format of 'name1,document1_id:access_key1;name2,document2_id:access_key2'.")
	} else {
		fmt.Println("Invalid command.")
	}
}

func peers(cmd []string, client serverpb.ClientClient, ctx context.Context) {
	if len(cmd) < 2 {
		fmt.Println("Incorrect number of arguments.")
	} else if cmd[1] == "list" {
		args := &serverpb.GetPeersRequest{}
		resp, err := client.GetPeers(ctx, args)
		if err != nil {
			fmt.Println(err)
		} else {
			fmt.Println(resp.GetPeers())
		}
	} else if cmd[1] == "add" && len(cmd) == 3 {
		args := &serverpb.AddPeerRequest{
			Addr: cmd[2],
		}
		resp, err := client.AddPeer(ctx, args)
		if err != nil {
			fmt.Println(err)
		} else {
			fmt.Println(resp)
		}
	} else if cmd[1] == "add" && len(cmd) != 3 {
		fmt.Println("Please specify a peer ID.")
	} else {
		fmt.Println("Invalid command.")
	}
}

func reference(cmd []string, client serverpb.ClientClient, ctx context.Context) {
	if len(cmd) < 3 {
		fmt.Println("Incorrect number of arguments.")
	} else if cmd[1] == "get" && len(cmd) != 3 {
		fmt.Println("Please specify a reference access ID.")
	} else if cmd[1] == "get" && len(cmd) == 3 {
		args := &serverpb.GetReferenceRequest{
			ReferenceId: cmd[2],
		}
		resp, err := client.GetReference(ctx, args)
		if err != nil {
			fmt.Println(err)
		} else {
			fmt.Println(resp.GetReference().GetValue())
		}
	} else if cmd[1] == "add" && len(cmd) == 4 {
		if !strings.Contains(cmd[2], "document@") && !strings.Contains(cmd[2], "reference@") {
			fmt.Println("Record should be in the format of 'document@document_id:access_key' or 'reference@reference_id:access_key'.")
			return
		}
		// Load private key into bytes
		var privateBody []byte
		privatePath := cmd[3]

		privateBody, err := ioutil.ReadFile(privatePath)
		if err != nil {
			fmt.Println(err)
			return
		}

		args := &serverpb.AddReferenceRequest{
			PrivKey: privateBody,
			Record:  cmd[2],
		}

		resp, err := client.AddReference(ctx, args)
		if err != nil {
			fmt.Println(err)
		} else {
			fmt.Println(resp.GetReferenceId())
		}

	} else if cmd[1] == "add" && len(cmd) != 4 {
		fmt.Println("Please specify a record and private key.")
	} else {
		fmt.Println("Invalid command.")
	}
}

func publish(cmd []string, client serverpb.ClientClient, ctx context.Context) {
	if len(cmd) != 3 {
		fmt.Println("Incorrect number of arguments.")
		return
	}
	var privateBody []byte
	privatePath := cmd[2]

	privateBody, err := ioutil.ReadFile(privatePath)
	if err != nil {
		fmt.Println(err)
		return
	}
	args := &serverpb.PublishRequest{
		PrivKey: privateBody,
		Message: cmd[1],
	}
	resp, err := client.Publish(ctx, args)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("Successfully published message with %d listeners. 🌎\n", resp.GetListeners())
}

func subscribe(cmd []string, client serverpb.ClientClient, ctx context.Context) {
	if len(cmd) != 2 {
		fmt.Println("Incorrect number of arguments.")
		return
	}
	args := &serverpb.SubscribeRequest{
		ChannelId: cmd[1],
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	stream, err := client.SubscribeClient(ctx, args)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("Listening for messages... Press enter to stop. 📡")

	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				fmt.Println(err)
				return
			}
			fmt.Println(msg.Message)
		}
	}()

	for {
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		if input != "" {
			return
		}
	}
}

func getContentType(fname string) string {
	return mime.TypeByExtension(filepath.Ext(fname))
}

func addDir(root string, ctx context.Context, client serverpb.ClientClient) (string, error) {
	file, err := os.Open(root)
	if err != nil {
		return "", err
	}
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		data, err := ioutil.ReadAll(file)
		if err != nil {
			return "", err
		}
		contentType := getContentType(root)
		args := &serverpb.AddRequest{
			Document: &serverpb.Document{
				Data:        data,
				ContentType: contentType,
			},
		}
		resp, err := client.Add(ctx, args)
		if err != nil {
			return "", err
		}
		return resp.GetAccessId(), nil
	}

	files, err := file.Readdirnames(0)
	if err != nil {
		return "", err
	}
	document := &serverpb.Document{
		ContentType: "directory",
		Children:    make(map[string]string),
	}
	for _, fname := range files {
		hash, err := addDir(filepath.Join(root, fname), ctx, client)
		if err != nil {
			return "", err
		}
		document.Children[filepath.Base(fname)] = hash
	}
	args := &serverpb.AddRequest{
		Document: document,
	}
	resp, err := client.Add(ctx, args)
	if err != nil {
		return "", nil
	}
	return resp.GetAccessId(), nil
}
