This sync package contains the code related to negentropy (event-set-reconciliation)
and using the BitTorrent / Mainline DHT for broadcast and discovery of relays.


## BEP44: BitTorrent DHT Token Management

BEP44 is a BitTorrent protocol that enables posting arbitrary signed data to hashes derived from a BitTorrent public key. 

### How It Works

1. **DHT Key**: Nestr users that supply a dht_key can declare their list of known relays (urls) to the DHT
2. **Target**: The input to the DHT for BEP44 is SHA1(pubkey + salt) where the salt is optional bytes (we use empty salt)
3. **Periodic Upload**: The RelayStore object periodically re-uploads the users' relay lists to the DHT, which holds it for about 2 hours 
4. **Relay Retrieval**: Other users can query the DHT using the dht_key to retrieve the relay list
5. **Negentropy Sync**: The RelayStore object periodically tries to sync with all relays that it is aware of
6. **Sync Store**: There is a sqlite db that stores info about relays for uploading to dht and syncing

For more detailed information, please refer to the official [BEP44 specification](https://www.bittorrent.org/beps/bep_0044.html).