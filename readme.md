# Hornet Relay
H.O.R.N.E.T.S. stands for **Hash Organized Relay Network Enabling Tamper-resistant Storage**. It is a decentralized off-chain storage system.

### bbolt: Stateful Buckets
bbolt provides support for bucketing, meaning content can be organized in an incredibly organized way. Bucketing is like a library. 📚 First, the librarian traverses the name of each book title (name of each bucket/database instance), then moves on to the pages (key-value pairs in that bucket) within the book. This traversal method is a lot quicker than skimming through every page in the library to find something specific. Nested databases (buckets) allow for this type of hierarchical data organization, so far more databases can be accessed concurrently, meaning traversal speeds may be faster even if bbolt doesn’t match-up to the raw speed of LMDB.

### Libp2p: Transport and Networking
Hornet Storage utilizes libp2p for its transport layers and networking, eliminating dependence on the centralized web. This forms the basis for the Hornet browser extension, similar to the [IPFS companion](https://github.com/ipfs/ipfs-companion) browser extension.

### Nostr Integration
Hornet storage operates beneath Nostr, storing relay data as signed Scionic Merkle DAGs in its key-value database. The relay manager lets operators view data usage per application over time periods, and it will also allow them to toggle which applications they want to host.

### Toggling Nostr Apps/Services
Data is tagged with a unique identifier indicating its type of application, such as "nostr note" for posts, "nostrtube" for a nostr YouTube video, "stemstr" for a stemstr music note, "git" for a git repository folder, et al.

## Disclaimer
⚠️ **Warning**: The Hornet relay is currently in its development phase. It is not yet stable and is not recommended for production use. Users are advised to exercise caution. More comprehensive information will be provided soon.
