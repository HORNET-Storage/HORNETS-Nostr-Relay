# Hornet Relay
H.O.R.N.E.T.S. stands for **Hash Organized Relay Network Enabling Tamper-resistant Storage**. It is a decentralized off-chain storage system.

### BadgerDB: SSD-focused Storage
The Hornet relay currently uses BadgerDB to focus on SSD storage. While SSDs might not be as fast as RAM (memory-mapped/LMDB), SSDs are still faster than HDDs. This means SSD are the middle-way and may actually be a balanced solution that's both economical for relay operators and offers better performance than HDDs, making the task of running a relay more manageable -- further promoting decentralization.

### Libp2p: Transport and Networking
Hornet utilizes Libp2p for its transport layers and networking, eliminating dependence on the centralized web. This forms the basis for the Hornet browser extension, similar to the IPFS companion browser extension.

### Nostr Integration
Hornet storage operates beneath Nostr, storing relay data as Merkle DAGs/Trees in its key-value database. The relay manager lets operators view data usage by service and select which services to host.

### Toggling Nostr Apps/Services
Data is tagged with a credential indicating its service type, such as "nostr note" for posts, "nostrtube" for a Nostr YouTube video, "stemstr" for a Stemstr music note, and "git" for a git repository folder, among others.

## Disclaimer
⚠️ **Warning**: The Hornet relay is currently in its development phase. It is not yet stable and is not recommended for production use. Users are advised to exercise caution. More comprehensive information will be provided soon.
