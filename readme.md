# H.O.R.N.E.T Storage: Multimedia Nostr Relay
H.O.R.N.E.T.S stands for **Hash Organized Relay Network Enabling Tamper-resistant Storage**. The Multimedia Nostr Relay supports chunked files with Scionic Merkle Trees and unchunked files with Blossom in a modular way, allowing operators to configure what they want to store.


# Supported NIPs
-	NIP-01: Basic Nostr Protocol
-	NIP-02 - Follow List
-	NIP-05 - DNS Identifier
-	NIP-09 - Event Deleting
-	NIP-11 - Relay Information Document
-	NIP-18 - Reposts
-	NIP-23 - Long-form Articles
-	NIP-25 - Custom Reactions
-	NIP-50 - Search Capability
-	NIP-51 - Custom Follow Lists
-	NIP-57 - Zaps

# Supported kinds
- kind0
- kind1
- kind3
- kind5
- kind6
- kind7
- kind8
- kind1984
- kind9372
- kind9373
- kind9375
- kind9802
- kind10000
- kind30000
- kind30008
- kind30009

### Toggle Kind Numbers and File Extensions
The HORNET Storage Relay Web Panel allows users to configure which types of nostr posts and file types they want to host.

### Getting Started
Example bat files are included for building and running a hornet-storage relay under a development environment

**DO NOT USE THESE FOR A PRODUCTION ENVIRONMENT**

The best example of how to use hornet storage can be found in the services/main.go file as this implementation can be used with any libp2p instance, the important take from that is the following.

```go
store := &stores_bbolt.BBoltStore{}

store.InitStore("main")

host, err := libp2p.New()

if err != nil {
  log.Fatal(err)
}

handlers.AddDownloadHandler(host, store, func(rootLeaf *merkle_dag.DagLeaf) bool {
  return true
})

handlers.AddUploadHandler(host, store, func(dag *merkle_dag.Dag) {
  
})
```

**Breakdown**

First we create a new store, supported stores can be found in lib/stores, and then we initialize it

```go
store := &stores_bbolt.BBoltStore{}

store.InitStore("main")
```

Then we can create our libp2p instance, I have ommitted out any configuration for this as it's not relevant for this documentation

**Please open an issue if any specific configuration is causing issues with the usage of the handlers, ensure to include the configuration**

```go
host, err := libp2p.New()
```

Now we can build and add the handlers for our libp2p instance using the following

```go
handlers.AddDownloadHandler(host, store, func(rootLeaf *merkle_dag.DagLeaf) bool {
  return true
})

handlers.AddUploadHandler(host, store, func(dag *merkle_dag.Dag) {
  
})
```
Make sure to pass in your store so the handlers know where to store and retrieve their leaves from

Only add the handlers you want your libp2p instance to support but both upload and download handlers are required if you want to support sending and receiving scionic-merkletrees over the network branch by branch

### Upload Handler
The upload handler requires an additional function to be passed in which allows you to add functionality when the handler completes and has the full verified dag which makes it easier to interact with a newly uploaded dag without having extra listening logic outside of the handler

### Download Handler
The download handler also requires an additional function that returns a boolean, this adds additional functionality to the start of the download handler which allows you to block a user from downloading specific data by returning false or allowing it by returning true. Additional data will eventually be added to this but for now the root hash is passed through. This is especially useful if you wanted to evaluate a users permissions based on their public key and the root hash etc.

## Disclaimer
⚠️ **Warning**: The Hornet relay is currently in its development phase. It is not yet stable and is not recommended for production use. Users are advised to exercise caution. More comprehensive information will be provided soon.
