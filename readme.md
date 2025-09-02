![nostr Badge](https://img.shields.io/badge/nostr-8e30eb?style=flat) ![Go Badge](https://img.shields.io/badge/Go-00ADD8?logo=go&logoColor=white) <img src="https://static.wixstatic.com/media/e9326a_3823e7e6a7e14488954bb312d11636da~mv2.png" height="20">

# H.O.R.N.E.T Storage Nostr Relay 🐝

Unleashing the power of Nostr with a ***configurable all-in-one relay*** supporting unchunked files as Blossom Blobs, chunked files as Scionic Merkle Trees, and various social media features as Nostr kind numbers.

### Choose Kind Numbers and File Extensions
Relay operators can select which file types and nostr features to enable in the [H.O.R.N.E.T Storage Relay Panel](https://github.com/HORNET-Storage/hornet-storage-panel) with elegant GUI toggles, displayed alongside diagrams and graphs to visualize the amount of data hosted over time.

### 17 Supported Nostr Features (NIPs)
**✅ - Implemented:** Features that are currently available and fully operational.
**⚠️ - In-Progress:** Features that are currently under development and not yet released.

| NIP Number | NIP Description                        | Kind Number Description                                                      |
|------------|------------------------------------|-------------------------------------------------------------------|
| NIP-01     | Basic Nostr Protocol               | [***kind0***](https://github.com/HORNET-Storage/hornet-storage/tree/main/lib/handlers/nostr/kind0) → User Metadata ✅<br><br>[***kind1***](https://github.com/HORNET-Storage/hornet-storage/tree/main/lib/handlers/nostr/kind1) → Short Text Post [Immutable] ✅ |
| NIP-02     | Following List                        | [***kind3***](https://github.com/HORNET-Storage/hornet-storage/tree/main/lib/handlers/nostr/kind3) → List of Users You Follow ✅                                         |
| NIP-05     | Mapping Nostr Address to DNS   | No Specific Kinds Listed ✅                                       |
| NIP-09     | Delete Note                        | [***kind5***](https://github.com/HORNET-Storage/hornet-storage/tree/main/lib/handlers/nostr/kind5) → Delete Request ✅                                         |
| NIP-11     | Relay Info Document                | No Specific Kinds Listed ✅                                       |
| NIP-18     | Reposts                            | [***kind6***](https://github.com/HORNET-Storage/hornet-storage/tree/main/lib/handlers/nostr/kind6) → Repost of Kind1 Notes ✅<br><br>[***kind16***](https://github.com/HORNET-Storage/hornet-storage/tree/main/lib/handlers/nostr/kind16) → Repost of All Other Kind Notes ✅ |
| NIP-23     | Formatted Articles                 | [***kind30023***](https://github.com/HORNET-Storage/hornet-storage/tree/main/lib/handlers/nostr/kind30023) → Markdown Post [Updatable] ✅                        |
| NIP-25     | Reactions                          | [***kind7***](https://github.com/HORNET-Storage/hornet-storage/tree/main/lib/handlers/nostr/kind7) → Like, Heart, or Custom Reaction ✅                        |
| NIP-45     | Counting Followers & more...          | No Specific Kinds Listed ✅                                       |
| NIP-50     | Search Capability                  | No Specific Kinds Listed ✅                                       |
| NIP-51     | Custom Lists                       | [***kind10000***](https://github.com/HORNET-Storage/hornet-storage/tree/main/lib/handlers/nostr/kind10000) → Mute List ✅<br><br>[***kind10001***](https://github.com/HORNET-Storage/hornet-storage/tree/main/lib/handlers/nostr/kind10001) → Pinned Note ✅<br><br>*kindxxxx* → Private Follow List [Encrypted] ⚠️<br><br>*kindxxxx* → Private Bookmark [Encrypted] ⚠️<br><br>[***kind30000***](https://github.com/HORNET-Storage/hornet-storage/tree/main/lib/handlers/nostr/kind30000) → Public Follow List [Unencrypted] ✅ |
| NIP-56     | Reporting                          | [***kind1984***](https://github.com/HORNET-Storage/hornet-storage/tree/main/lib/handlers/nostr/kind1984) → Report a User, Post, or Relay ✅                       |
| NIP-57     | Lightning Zaps                     | [***kind9735***](https://github.com/HORNET-Storage/hornet-storage/tree/main/lib/handlers/nostr/kind9735) → Lightning Zap Receipt ✅                                         |
| NIP-58     | Badges                             | [***kind8***](https://github.com/HORNET-Storage/hornet-storage/tree/main/lib/handlers/nostr/kind8) → Badge Award ✅<br><br>[***kind30008***](https://github.com/HORNET-Storage/hornet-storage/tree/main/lib/handlers/nostr/kind30008) → Profile Badge ✅<br><br>[***kind30009***](https://github.com/HORNET-Storage/hornet-storage/tree/main/lib/handlers/nostr/kind30009) → Badge Definition ✅ |
| NIP-65     | Propagate Tiny Relay Lists         | [***kind10002***](https://github.com/HORNET-Storage/hornet-storage/tree/main/lib/handlers/nostr/kind10002) → Tiny Relay List [Outbox Model] ✅                          |
| NIP-84     | Highlights                         | [***kind9802***](https://github.com/HORNET-Storage/hornet-storage/tree/main/lib/handlers/nostr/kind9802) → Snippet of a Post or Article ✅                       |
| NIP-116    | Event Paths                        | [***kind30079***](https://github.com/HORNET-Storage/hornet-storage/tree/main/lib/handlers/nostr/kind30079) → Paths Instead of Kind Numbers ✅                     |



## ⚙️ Developer Requirements & Build Instructions

### 📦 **System Requirements**

To build and run HORNETS-Nostr-Relay from source, ensure the following tools are installed:

✅ **Go 1.22+**
Official Go programming language environment. Download from:
[https://golang.org/dl/](https://golang.org/dl/)

✅ **GCC (GNU Compiler Collection)**
Required for building C-based dependencies via `cgo`.

----

#### If On **Linux or Debian** Then Run:

  ```bash
  sudo apt update
  sudo apt install build-essential
  ```

#### If On **macOS** Then Run:

  ```bash
  xcode-select --install
  ```

#### If On **Windows** Then Run:
  Recommended: [MSYS2](https://www.msys2.org/)

  ```bash
  pacman -S base-devel gcc
  ```

  Alternatively: [MinGW-w64](https://www.mingw-w64.org/downloads/)

---

### 🚀 **Building Relay with Panel** (When Needing to Pull Latest Panel Version)

After cloning the repository,

```bash
git clone https://github.com/HORNET-Storage/HORNETS-Nostr-Relay.git
cd HORNETS-Nostr-Relay
```

#### On **Linux or macOS**:

*Run this script found in the main directory:*
```bash
./build-panel.sh
```

#### On **Windows**:

*Run this script found in the main directory:*
```powershell
./build-panel.bat
```

---


### 🚀 **Building Relay with Panel** (Hot Reload Dev Mode If Modifying Panel In Subfolder /panel-source):

After cloning the repository,

```bash
git clone https://github.com/HORNET-Storage/HORNETS-Nostr-Relay.git
cd HORNETS-Nostr-Relay
```

#### On **Linux or macOS**:

*Run this script found in the main directory:*
```bash
./build-panel-devmode.sh
```

#### On **Windows**:

*Run this script found in the main directory:*
```powershell
./build-panel-devmode.bat
```

The compiled binary (`hornet-storage` or `hornet-storage.exe`) will be created in the project root directory.

#### ⚠️ When Troubleshooting:

*Make sure the port in the .env.development file for the relay's base URL matches the port that the relay is using inside of the config.yaml file.*

***Web panel is always served on that port +2, so if the relay is running on 9000 then the .env.development base url needs to point to 9002.***

---

### 🚀 **Building Relay without Panel**

After cloning the repository,

```bash
git clone https://github.com/HORNET-Storage/HORNETS-Nostr-Relay.git
cd HORNETS-Nostr-Relay
```

##### If On **Linux or macOS** Then Run:

*Run this script found in the main directory:*
```bash
./build.sh
```

##### If On **Windows** Then Run:

*Run this script found in the main directory:*
```powershell
./build.bat
```

The compiled binary (`hornet-storage` or `hornet-storage.exe`) will be created in the project root directory.

---

### 🔑 **Configuration Setup**

On first run the relay will automatically generate a config.yaml with a default configuration and a new private key which makes getting started nice and easy.

There are also example configs included for specific situations.
The config.example.dev has all content moderation disabled and allows all kinds and users with no restrictions.

You can copy and rename manually or use the following if you wish to use any of the example configurations.

#### **Bash**
```bash
cp config.example.dev.yaml config.yaml
```

#### **Powershell**
```powershell
copy config.example.dev.yaml config.yaml
```

If copying an example config make sure to update the private key.

Set the `private_key` field to a valid Nostr private key in either **nsec bech32 format** or **hexadecimal format**. This key identifies your relay on the Nostr network and is required for operation.


### Additional Services

The relay is designed to run with optional services along side it and those can be found here:

[Super Neutrino Wallet](https://github.com/HORNET-Storage/Super-Neutrino-Wallet)
- Paid relay features using a bitcoin wallet

[NestShield](https://github.com/HORNET-Storage/NestShield)
- Content moderation using python

[Ollama](https://ollama.com/download)
- More advanced and resource intensive content moderation

---

## Disclaimer ##
**WARNING**: Relay is still being developed and is not ready for production use yet. More details will be provided soon.
