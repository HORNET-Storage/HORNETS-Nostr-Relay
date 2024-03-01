# H.O.R.N.E.T Storage: Multimedia Nostr Relay
H.O.R.N.E.T.S stands for **Hash Organized Relay Network Enabling Tamper-resistant Storage**. The Multimedia Nostr Relay supports chunked file storage with Scionic Merkle Trees and is equipped with Libp2p to sync with other Multimedia Nostr Relays, free of centralized certificate authories.

### bbolt: Stateful Buckets
bbolt provides support for bucketing, meaning content can be organized in an incredibly organized way. Bucketing is like a library. 📚 First, the librarian traverses the name of each book title (name of each bucket/database instance), then moves on to the pages (key-value pairs in that bucket) within the book. This traversal method is a lot quicker than skimming through every page in the library to find something specific. Nested databases (buckets) allow for this type of hierarchical data organization, so far more databases can be accessed concurrently, meaning traversal speeds may be faster even if bbolt doesn’t match-up to the raw speed of LMDB.

### Libp2p: Transport and Networking
H.O.R.N.E.T Storage utilizes libp2p for its transport layers and networking, eliminating dependence on the centralized web. This forms the basis for the Hornet browser extension, similar to the [IPFS companion](https://github.com/ipfs/ipfs-companion) browser extension.

### Toggling Nostr Apps/Services
Data is tagged with a unique identifier indicating its type of application, such as "nostr note" for posts, "nostrtube" for a nostr YouTube video, "stemstr" for a stemstr music note, "git" for a git repository folder, et al. The web-based relay manager will allow relay operators to view data usage per application over periods of time, and it will also allow them to toggle which file types they want to host.

### Getting Started
Example bat files are included for building and running a hornet-storage relay under a development environment

**DO NOT USES THESE FOR A PRODUCTION ENVIRONMENT**

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
