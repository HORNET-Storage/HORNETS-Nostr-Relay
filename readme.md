![nostr Badge](https://img.shields.io/badge/nostr-8e30eb?style=flat) ![Go Badge](https://img.shields.io/badge/Go-00ADD8?logo=go&logoColor=white) <img src="https://static.wixstatic.com/media/e9326a_3823e7e6a7e14488954bb312d11636da~mv2.png" height="20">

# H.O.R.N.E.T Storage Nostr Relay 🐝

Unleashing the power of Nostr with a ***configurable all-in-one relay*** supporting unchunked files as Blossom Blobs, chunked files as Scionic Merkle Trees, and various social media features as Nostr kind numbers.

### Choose Kind Numbers and File Extensions
Select which file types and nostr features to enable in the [H.O.R.N.E.T Storage Relay Panel](https://github.com/HORNET-Storage/hornet-storage-panel) with elegant GUI toggles, displayed alongside diagrams and graphs to visualize the amount of data hosted over time.

### Multi-OS Go Nostr Relay - Currently Supports 15 NIPs (Nostr Features):
**✅ - Implemented:** Features that are currently available and fully operational.  
**⚠️ - In-Progress:** Features that are currently under development and not yet released.

| NIP Number | Description                        | Kind Numbers                                                      |
|------------|------------------------------------|-------------------------------------------------------------------|
| NIP-01     | Basic Nostr Protocol               | kind0 → User Metadata ✅<br><br>kind1 → Short Text Post [Immutable] ✅ |
| NIP-02     | Follow List                        | kind3 → Following List ✅                                         |
| NIP-05     | Mapping Nostr Public Keys to DNS   | No Specific Kinds Listed ✅                                       |
| NIP-09     | Delete Note                        | kind5 → Delete Request ✅                                         |
| NIP-11     | Relay Info Document                | No Specific Kinds Listed ✅                                       |
| NIP-18     | Reposts                            | kind6 → Repost of Kind1 Notes ✅<br><br>kind16 → Repost of All Other Kind Notes ✅ |
| NIP-23     | Formatted Articles                 | kind30023 → Markdown Post [Replaceable] ✅                        |
| NIP-25     | Reactions                          | kind7 → Like, Heart, or Custom Reaction ✅                        |
| NIP-50     | Search Capability                  | No Specific Kinds Listed ✅                                       |
| NIP-51     | Custom Lists                       | kind10000 → Mute List ✅<br><br>kind10001 → Pinned Notes ✅<br><br>kind29998 → Private Follow Lists [Encrypted] ⚠️<br><br>kind29999 → Private Bookmarks [Encrypted] ⚠️<br><br>kind30000 → Public Follow Lists [Unencrypted] ✅ |
| NIP-56     | Reporting                          | kind1984 → Report a User, Post, or Relay ✅                       |
| NIP-57     | Lightning Zaps                     | kind9735 → Zap Receipt ✅                                         |
| NIP-58     | Badges                             | kind8 → Badge Award ✅<br><br>kind30008 → Profile Badges ✅<br><br>kind30009 → Badge Definition ✅ |
| NIP-84     | Highlights                         | kind9802 → Snippets of Posts or Articles ✅                       |
| NIP-116    | Event Paths                        | kind30079 → Paths Instead of Kind Numbers ⚠️                      |

## Disclaimer
**WARNING**: Relay is still being developed and is not ready for production use yet. More details will be provided soon.
