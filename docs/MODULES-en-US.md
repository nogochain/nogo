# NogoChain Key Technical Modules Documentation

**Version**: 1.0  
**Generated Date**: 2026-04-01  
**Language**: English (US)

---

## Table of Contents

1. [Wallet Implementation](#1-wallet-implementation)
2. [Transaction Mechanism](#2-transaction-mechanism)
3. [Block Structure](#3-block-structure)
4. [Network Protocol](#4-network-protocol)
5. [Smart Contracts](#5-smart-contracts)
6. [AI Audit](#6-ai-audit)
7. [Governance and Upgrades](#7-governance-and-upgrades)
8. [Monitoring Metrics](#8-monitoring-metrics)

---

## 1. Wallet Implementation

### 1.1 Account Model

#### 1.1.1 Ed25519 Key Pair

NogoChain uses the Ed25519 digital signature algorithm, providing high security and performance.

**Core Structure**:

```go
type Wallet struct {
    PrivateKey ed25519.PrivateKey  // Ed25519 private key (64 bytes)
    PublicKey  ed25519.PublicKey   // Ed25519 public key (32 bytes)
    Address    string              // Address (hex-encoded SHA256 of public key)
}
```

**Address Generation Algorithm**:

```go
func GenerateAddress(pubKey ed25519.PublicKey) string {
    sum := sha256.Sum256(pubKey)
    return hex.EncodeToString(sum[:])  // 64-character hexadecimal string
}
```

**Code Example - Creating a New Wallet**:

```go
// Generate random key pair
pub, priv, err := ed25519.GenerateKey(rand.Reader)
if err != nil {
    return nil, err
}

// Create wallet
wallet := &Wallet{
    PrivateKey: priv,
    PublicKey:  pub,
    Address:    GenerateAddress(pub),
}
```

#### 1.1.2 Address Format

- **Format**: 64-character hexadecimal string
- **Example**: `NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf`
- **Validation**: Supports two format validations
  - Raw hexadecimal: Direct hexadecimal encoding validation
  - NOGO00 prefix format: Validate prefix then remaining part

### 1.2 HD Wallet (Hierarchical Deterministic Wallet)

#### 1.2.1 BIP39 Mnemonic

**Core Parameters**:

```go
const (
    MnemonicWordCount      = 12  // Number of mnemonic words
    MnemonicEntropyBits    = 128 // Entropy size (128 bits)
    MnemonicChecksumBits   = 4   // Checksum size (128/32)
)
```

**Mnemonic Generation Flow**:

```
1. Generate 128-bit random entropy (16 bytes)
2. Calculate SHA256 checksum (first 4 bits)
3. Entropy + checksum = 132 bits
4. Map every 11 bits to a word (12 words total)
5. Use BIP39 English wordlist (2048 words)
```

**Code Example - Generating Mnemonic**:

```go
func GenerateMnemonic() (string, error) {
    // Generate random entropy
    entropy := make([]byte, 16)
    _, err := rand.Read(entropy)
    if err != nil {
        return "", fmt.Errorf("failed to generate entropy: %w", err)
    }
    
    return EntropyToMnemonic(entropy)
}

// Entropy to mnemonic conversion
func EntropyToMnemonic(entropy []byte) (string, error) {
    // Calculate checksum
    hash := sha256.Sum256(entropy)
    
    // Create bit array (entropy + checksum)
    totalBits := 128 + 4
    bitArray := make([]bool, totalBits)
    
    // Copy entropy bits
    for i := 0; i < 128; i++ {
        byteIndex := i / 8
        bitIndex := 7 - (i % 8)
        bitArray[i] = (entropy[byteIndex] & (1 << bitIndex)) != 0
    }
    
    // Copy checksum bits
    for i := 0; i < 4; i++ {
        bitArray[128+i] = (hash[0] & (1 << (7 - i))) != 0
    }
    
    // Convert every 11 bits to a word index
    words := make([]string, 12)
    for i := 0; i < 12; i++ {
        index := uint16(0)
        for j := 0; j < 11; j++ {
            if bitArray[i*11+j] {
                index |= (1 << (10 - j))
            }
        }
        words[i] = wordlist[index]
    }
    
    return strings.Join(words, " "), nil
}
```

**Mnemonic Validation**:

```go
func ValidateMnemonic(mnemonic string) bool {
    _, err := MnemonicToEntropy(mnemonic)
    return err == nil
}

// Mnemonic to entropy (validate checksum)
func MnemonicToEntropy(mnemonic string) ([]byte, error) {
    words := strings.Fields(mnemonic)
    
    if len(words) != 12 {
        return nil, fmt.Errorf("invalid word count: %d", len(words))
    }
    
    // Validate each word and convert to index
    indices := make([]uint16, 12)
    for i, word := range words {
        found := false
        for j, w := range wordlist {
            if w == word {
                indices[i] = uint16(j)
                found = true
                break
            }
        }
        if !found {
            return nil, fmt.Errorf("invalid word: %s", word)
        }
    }
    
    // Convert to bit array and validate checksum
    // ...
}
```

#### 1.2.2 BIP32 Derivation

**HD Wallet Structure**:

```go
type HDWallet struct {
    PrivateKey ed25519.PrivateKey  // Current level private key
    PublicKey  ed25519.PublicKey   // Current level public key
    ChainCode  []byte              // Chain code (32 bytes)
    Depth      uint8               // Depth (starting from 0)
    Index      uint32              // Index number
    Parent     []byte              // Parent key fingerprint
}
```

**Derivation Path Parsing**:

```go
// Support standard path format: m/44'/0'/0'/0/0
func parsePath(path string) []uint32 {
    var result []uint32
    
    // Remove "m/" prefix
    if len(path) >= 2 && path[:2] == "m/" {
        path = path[2:]
    }
    
    // Parse each level
    segments := splitPath(path)
    for _, seg := range segments {
        isHardened := len(seg) > 0 && seg[len(seg)-1] == '\''
        var index uint32
        
        if isHardened {
            // Hardened derivation: index = normal index + 0x80000000
            numStr := seg[:len(seg)-1]
            fmt.Sscanf(numStr, "%d", &index)
            index += BIP32HardenedBase  // 0x80000000
        } else {
            // Normal derivation
            fmt.Sscanf(seg, "%d", &index)
        }
        result = append(result, index)
    }
    
    return result
}
```

**Key Derivation Algorithm**:

```go
func (w *HDWallet) Derive(path string) (*HDWallet, error) {
    segments := parsePath(path)
    depth := uint8(0)
    
    for _, seg := range segments {
        isHardened := seg >= BIP32HardenedBase
        index := seg
        if isHardened {
            index = seg - BIP32HardenedBase
        }
        depth++
        
        // Prepare derivation data
        data := make([]byte, 37)
        if isHardened {
            // Hardened derivation: 0x00 + private key
            data[0] = 0x00
            copy(data[1:33], w.PrivateKey[32:])
        } else {
            // Normal derivation: public key
            copy(data, w.PublicKey)
        }
        binary.BigEndian.PutUint32(data[33:37], index)
        
        // HMAC-SHA512(ChainCode, Data)
        hmac := sha512.New()
        hmac.Write(w.ChainCode)
        hmac.Write(data)
        I := hmac.Sum(nil)
        
        // Split result
        childKey := make([]byte, 32)
        childChainCode := make([]byte, 32)
        copy(childKey, I[:32])
        copy(childChainCode, I[32:64])
        
        // Child private key = parent private key + IL (mod n)
        for j := range childKey {
            childKey[j] ^= w.PrivateKey[j]
        }
        
        // Create child wallet
        wallet := &HDWallet{
            PrivateKey: ed25519.PrivateKey(childKey),
            PublicKey:  ed25519.PrivateKey(childKey).Public().(ed25519.PublicKey),
            ChainCode:  childChainCode,
            Depth:      depth,
            Index:      index,
            Parent:     parentFingerprint,
        }
        
        w = wallet
    }
    
    return w, nil
}
```

**Creating HD Wallet from Mnemonic**:

```go
func NewHDWallet(seed []byte) (*HDWallet, error) {
    // BIP39 seed to HD wallet seed
    // Use HMAC-SHA512("ed25519 seed", seed)
    hmac := sha512.New()
    hmac.Write([]byte("ed25519 seed"))
    hmac.Write(seed)
    I := hmac.Sum(nil)
    
    // IL as master private key, IR as master chain code
    key := make([]byte, 32)
    copy(key, I[:32])
    chainCode := make([]byte, 32)
    copy(chainCode, I[32:64])
    
    return &HDWallet{
        PrivateKey: ed25519.PrivateKey(key),
        PublicKey:  ed25519.PrivateKey(key).Public().(ed25519.PublicKey),
        ChainCode:  chainCode,
        Depth:      0,
        Index:      0,
        Parent:     nil,
    }, nil
}
```

#### 1.2.3 Mnemonic to Seed

```go
func MnemonicToSeed(mnemonic, passphrase string) ([]byte, error) {
    // Normalize mnemonic (NFKD normalization)
    normalizedMnemonic := strings.ToLower(strings.TrimSpace(mnemonic))
    
    // Create salt: "mnemonic" + passphrase
    salt := "mnemonic" + passphrase
    
    // PBKDF2 with HMAC-SHA512 (2048 iterations)
    hmac := sha512.New()
    hmac.Write([]byte(salt))
    hmac.Write([]byte(normalizedMnemonic))
    seed := hmac.Sum(nil)
    
    // Iterate 2048 times
    for i := 0; i < 2047; i++ {
        hmac.Reset()
        hmac.Write(seed)
        seed = hmac.Sum(nil)
    }
    
    return seed, nil  // Return 64-byte seed
}
```

### 1.3 Key Management

#### 1.3.1 Encrypted Storage (Keystore)

**Keystore File Structure**:

```go
type KeystoreFile struct {
    Version   int               `json:"version"`   // Version number (currently 1)
    Address   string            `json:"address"`   // Wallet address
    KDF       KeystoreKDF       `json:"kdf"`       // Key derivation function parameters
    Cipher    KeystoreCipher    `json:"cipher"`    // Encryption parameters
    EncryptedMnemonic *EncryptedMnemonic `json:"encryptedMnemonic,omitempty"` // Optional: encrypted mnemonic
}

type KeystoreKDF struct {
    Name       string `json:"name"`       // KDF name ("pbkdf2-sha256")
    Iterations int    `json:"iterations"` // Iteration count (default 600,000)
    SaltB64    string `json:"saltB64"`    // Base64-encoded salt (16 bytes)
}

type KeystoreCipher struct {
    Name          string `json:"name"`          // Encryption algorithm ("aes-256-gcm")
    NonceB64      string `json:"nonceB64"`      // Base64-encoded nonce (12 bytes)
    CiphertextB64 string `json:"ciphertextB64"` // Base64-encoded ciphertext
}
```

**Encryption Flow**:

```go
func WriteKeystore(path string, w *Wallet, password string, params KeystoreKDF) error {
    // 1. Generate random salt
    salt := make([]byte, 16)
    _, err := rand.Read(salt)
    
    // 2. PBKDF2-HMAC-SHA256 key derivation
    key := pbkdf2HMACSHA256([]byte(password), salt, params.Iterations, 32)
    
    // 3. AES-256-GCM encryption
    block, err := aes.NewCipher(key)
    gcm, err := cipher.NewGCM(block)
    nonce := make([]byte, gcm.NonceSize())
    _, err = rand.Read(nonce)
    
    // 4. Bind address to prevent swap attacks
    addrBytes, _ := hex.DecodeString(w.Address)
    ciphertext := gcm.Seal(nil, nonce, []byte(w.PrivateKey), addrBytes)
    
    // 5. Write to JSON file
    ks := KeystoreFile{
        Version: 1,
        Address: w.Address,
        KDF: KeystoreKDF{
            Name:       "pbkdf2-sha256",
            Iterations: params.Iterations,
            SaltB64:    base64.StdEncoding.EncodeToString(salt),
        },
        Cipher: KeystoreCipher{
            Name:          "aes-256-gcm",
            NonceB64:      base64.StdEncoding.EncodeToString(nonce),
            CiphertextB64: base64.StdEncoding.EncodeToString(ciphertext),
        },
    }
    
    // Atomic file write
    tmp := path + ".tmp"
    os.WriteFile(tmp, json.MarshalIndent(ks, "", "  "), 0o600)
    os.Rename(tmp, path)
}
```

**Decryption Flow**:

```go
func WalletFromKeystore(path string, password string) (*Wallet, error) {
    // 1. Read keystore file
    ks, err := ReadKeystore(path)
    
    // 2. Validate version and parameters
    if ks.Version != 1 {
        return nil, fmt.Errorf("unsupported version: %d", ks.Version)
    }
    
    // 3. Decode parameters
    salt, _ := base64.StdEncoding.DecodeString(ks.KDF.SaltB64)
    nonce, _ := base64.StdEncoding.DecodeString(ks.Cipher.NonceB64)
    ciphertext, _ := base64.StdEncoding.DecodeString(ks.Cipher.CiphertextB64)
    
    // 4. Derive decryption key
    key := pbkdf2HMACSHA256([]byte(password), salt, ks.KDF.Iterations, 32)
    
    // 5. AES-GCM decryption
    block, _ := aes.NewCipher(key)
    gcm, _ := cipher.NewGCM(block)
    addrBytes, _ := hex.DecodeString(ks.Address)
    plain, err := gcm.Open(nil, nonce, ciphertext, addrBytes)
    
    if err != nil {
        return nil, ErrKeystoreWrongPassword
    }
    
    // 6. Create wallet from private key bytes
    return WalletFromPrivateKeyBytes(plain)
}
```

**PBKDF2-HMAC-SHA256 Implementation**:

```go
func pbkdf2HMACSHA256(password, salt []byte, iter, keyLen int) []byte {
    hLen := 32  // SHA256 output length
    numBlocks := (keyLen + hLen - 1) / hLen
    out := make([]byte, 0, numBlocks*hLen)
    var intBuf [4]byte
    
    for block := 1; block <= numBlocks; block++ {
        // T1 = U1
        binaryBigEndianPutUint32(intBuf[:], uint32(block))
        u := hmacSHA256(password, append(append([]byte(nil), salt...), intBuf[:]...))
        t := make([]byte, len(u))
        copy(t, u)
        
        // T = T1 XOR T2 XOR ... XOR Tc
        for i := 1; i < iter; i++ {
            u = hmacSHA256(password, u)
            for j := 0; j < len(t); j++ {
                t[j] ^= u[j]
            }
        }
        out = append(out, t...)
    }
    return out[:keyLen]
}
```

#### 1.3.2 Mnemonic Backup

**Encrypted Mnemonic Storage**:

```go
func WriteKeystoreWithMnemonic(path string, w *Wallet, mnemonic, password string, params KeystoreKDF) error {
    // 1. Write base keystore
    WriteKeystore(path, w, password, params)
    
    // 2. Encrypt mnemonic separately (with different salt)
    salt := make([]byte, 16)
    rand.Read(salt)
    
    mnemonicKey := pbkdf2HMACSHA256([]byte(password), salt, params.Iterations, 32)
    
    block, _ := aes.NewCipher(mnemonicKey)
    gcm, _ := cipher.NewGCM(block)
    nonce := make([]byte, gcm.NonceSize())
    rand.Read(nonce)
    
    ciphertext := gcm.Seal(nil, nonce, []byte(mnemonic), nil)
    
    // 3. Store in keystore
    ks, _ := ReadKeystore(path)
    ks.EncryptedMnemonic = &EncryptedMnemonic{
        SaltB64:      base64.StdEncoding.EncodeToString(salt),
        NonceB64:     base64.StdEncoding.EncodeToString(nonce),
        CiphertextB64: base64.StdEncoding.EncodeToString(ciphertext),
    }
    
    // 4. Atomic write
    tmp := path + ".tmp"
    os.WriteFile(tmp, json.MarshalIndent(ks, "", "  "), 0o600)
    os.Rename(tmp, path)
}
```

**Export Mnemonic**:

```go
func ExportMnemonicFromKeystore(path string, password string) (string, error) {
    ks, err := ReadKeystore(path)
    if err != nil {
        return "", err
    }
    
    if ks.EncryptedMnemonic == nil {
        return "", errors.New("no mnemonic stored")
    }
    
    // Decrypt mnemonic
    salt, _ := base64.StdEncoding.DecodeString(ks.EncryptedMnemonic.SaltB64)
    mnemonicKey := pbkdf2HMACSHA256([]byte(password), salt, ks.KDF.Iterations, 32)
    
    block, _ := aes.NewCipher(mnemonicKey)
    gcm, _ := cipher.NewGCM(block)
    
    nonce, _ := base64.StdEncoding.DecodeString(ks.EncryptedMnemonic.NonceB64)
    ciphertext, _ := base64.StdEncoding.DecodeString(ks.EncryptedMnemonic.CiphertextB64)
    
    plain, err := gcm.Open(nil, nonce, ciphertext, nil)
    if err != nil {
        return "", ErrKeystoreWrongPassword
    }
    
    return string(plain), nil
}
```

### 1.4 Wallet Operations

#### 1.4.1 Creating Wallets

```go
// Method 1: Random creation
func NewWallet() (*Wallet, error) {
    pub, priv, err := ed25519.GenerateKey(rand.Reader)
    return &Wallet{
        PrivateKey: priv,
        PublicKey:  pub,
        Address:    GenerateAddress(pub),
    }, nil
}

// Method 2: From mnemonic
func CreateWalletFromMnemonic(mnemonic, passphrase string) (*Wallet, string, error) {
    // Generate new mnemonic if not provided
    if mnemonic == "" {
        mnemonic, err = GenerateMnemonic()
    }
    
    // Validate mnemonic
    if !ValidateMnemonic(mnemonic) {
        return nil, "", errors.New("invalid mnemonic")
    }
    
    // Mnemonic to seed
    seed, err := MnemonicToSeed(mnemonic, passphrase)
    
    // Create HD wallet and derive
    hdWallet, _ := NewHDWallet(seed)
    derived, _ := hdWallet.Derive("m/0")
    
    return &Wallet{
        PrivateKey: derived.PrivateKey,
        PublicKey:  derived.PublicKey,
        Address:    GenerateAddress(derived.PublicKey),
    }, mnemonic, nil
}
```

#### 1.4.2 Importing Wallets

```go
// Import from Base64 private key
func WalletFromPrivateKeyBase64(privB64 string) (*Wallet, error) {
    raw, err := base64.StdEncoding.DecodeString(privB64)
    if err != nil {
        return nil, err
    }
    if len(raw) != ed25519.PrivateKeySize {
        return nil, errors.New("invalid private key length")
    }
    
    priv := ed25519.PrivateKey(raw)
    pub := priv.Public().(ed25519.PublicKey)
    
    return &Wallet{
        PrivateKey: priv,
        PublicKey:  pub,
        Address:    GenerateAddress(pub),
    }, nil
}

// Import from keystore
func WalletFromKeystore(path string, password string) (*Wallet, error) {
    // See section 1.3.1
}
```

#### 1.4.3 Exporting Wallets

```go
// Export Base64 private key
func (w *Wallet) PrivateKeyBase64() string {
    return base64.StdEncoding.EncodeToString(w.PrivateKey)
}

// Export Base64 public key
func (w *Wallet) PublicKeyBase64() string {
    return base64.StdEncoding.EncodeToString(w.PublicKey)
}

// Export mnemonic (only when created from mnemonic)
func WalletToMnemonic(w *Wallet) (string, error) {
    return "", errors.New("cannot export mnemonic: wallet was not created from mnemonic")
}
```

#### 1.4.4 Transaction Signing

```go
func (w *Wallet) SignTransaction(tx *Transaction) error {
    // 1. Calculate transaction hash
    hash, err := tx.SigningHash()
    if err != nil {
        return err
    }
    
    // 2. Ed25519 signature
    signature := ed25519.Sign(w.PrivateKey, hash)
    
    // 3. Set signature and public key
    tx.Signature = signature
    tx.FromPubKey = w.PublicKey
    
    return nil
}
```

---

## 2. Transaction Mechanism

### 2.1 Transaction Structure

#### 2.1.1 Transaction Type Definition

```go
type TransactionType string

const (
    TxCoinbase TransactionType = "coinbase"  // Block reward transaction
    TxTransfer TransactionType = "transfer"  // Transfer transaction
)

type Transaction struct {
    Type       TransactionType `json:"type"`        // Transaction type
    ChainID    uint64          `json:"chainId"`     // Chain ID (replay attack protection)
    
    FromPubKey []byte          `json:"fromPubKey,omitempty"`  // Sender public key (Base64)
    ToAddress  string          `json:"toAddress"`             // Recipient address
    
    Amount     uint64          `json:"amount"`      // Transfer amount
    Fee        uint64          `json:"fee"`         // Transaction fee
    Nonce      uint64          `json:"nonce,omitempty"`       // Sender transaction sequence number
    
    Data       string          `json:"data,omitempty"`        // Additional data (smart contract calls, etc.)
    Signature  []byte          `json:"signature,omitempty"`   // Signature (Base64)
}
```

#### 2.1.2 Account State

```go
type Account struct {
    Balance uint64 `json:"balance"`  // Account balance
    Nonce   uint64 `json:"nonce"`    // Used transaction sequence number
}
```

### 2.2 Transaction Signing

#### 2.2.1 Signing Hash Calculation (Consensus Layer)

**Dual Signing Hash Mechanism**:

```go
// Consensus layer signing hash (supports fork upgrades)
func txSigningHashForConsensus(tx Transaction, p ConsensusParams, height uint64) ([]byte, error) {
    if p.BinaryEncodingActive(height) {
        // Binary encoding (new version)
        return txSigningHashBinary(tx)
    }
    // JSON encoding (legacy version, backward compatible)
    return tx.signingHashLegacyJSON()
}
```

**JSON Encoding Hash (Legacy)**:

```go
func (t Transaction) signingHashLegacyJSON() ([]byte, error) {
    type signingView struct {
        Type      TransactionType `json:"type"`
        ChainID   uint64          `json:"chainId"`
        FromAddr  string          `json:"fromAddr,omitempty"`
        ToAddress string          `json:"toAddress"`
        Amount    uint64          `json:"amount"`
        Fee       uint64          `json:"fee"`
        Nonce     uint64          `json:"nonce,omitempty"`
        Data      string          `json:"data,omitempty"`
    }
    
    v := signingView{
        Type:      t.Type,
        ChainID:   t.ChainID,
        ToAddress: t.ToAddress,
        Amount:    t.Amount,
        Fee:       t.Fee,
        Nonce:     t.Nonce,
        Data:      t.Data,
    }
    
    // Transfer transactions include sender address
    if t.Type == TxTransfer {
        fromAddr, _ := t.FromAddress()
        v.FromAddr = fromAddr
    }
    
    // SHA256 after JSON serialization
    b, err := json.Marshal(v)
    if err != nil {
        return nil, err
    }
    sum := sha256.Sum256(b)
    return sum[:], nil
}
```

**Transaction ID Calculation**:

```go
func TxIDHex(tx Transaction) (string, error) {
    h, err := tx.SigningHash()
    if err != nil {
        return "", err
    }
    return hex.EncodeToString(h), nil
}

// Consensus layer transaction ID
func TxIDHexForConsensus(tx Transaction, p ConsensusParams, height uint64) (string, error) {
    h, err := txSigningHashForConsensus(tx, p, height)
    if err != nil {
        return "", err
    }
    return hex.EncodeToString(h), nil
}
```

#### 2.2.2 Ed25519 Signature

```go
// Sign transaction
func (w *Wallet) SignTransaction(tx *Transaction) error {
    // 1. Calculate signing hash
    hash, err := tx.SigningHash()
    if err != nil {
        return err
    }
    
    // 2. Ed25519 signature (64 bytes)
    signature := ed25519.Sign(w.PrivateKey, hash)
    
    // 3. Set signature and public key
    tx.Signature = signature
    tx.FromPubKey = w.PublicKey
    
    return nil
}

// Verify signature
func (t Transaction) verifyWithSigningHash(h []byte) error {
    if t.Type != TxTransfer {
        return errors.New("signature verification only for transfer transactions")
    }
    
    // Validate public key length
    if len(t.FromPubKey) != ed25519.PublicKeySize {
        return fmt.Errorf("invalid fromPubKey length: %d", len(t.FromPubKey))
    }
    
    // Validate signature length
    if len(t.Signature) != ed25519.SignatureSize {
        return fmt.Errorf("invalid signature length: %d", len(t.Signature))
    }
    
    // Ed25519 verification
    if !ed25519.Verify(t.FromPubKey, h, t.Signature) {
        return errors.New("invalid signature")
    }
    
    return nil
}
```

### 2.3 Transaction Verification

#### 2.3.1 Basic Verification

```go
func (t Transaction) Verify() error {
    switch t.Type {
    case TxCoinbase:
        // Validate coinbase transaction
        if t.ChainID == 0 {
            return errors.New("chainId must be set")
        }
        if t.Amount == 0 {
            return errors.New("coinbase amount must be > 0")
        }
        if err := validateAddress(t.ToAddress); err != nil {
            return fmt.Errorf("invalid toAddress: %w", err)
        }
        // Coinbase must not have signature/public key/nonce/fee
        if t.FromPubKey != nil || t.Signature != nil || t.Nonce != 0 || t.Fee != 0 {
            return errors.New("coinbase must not include fromPubKey/signature/nonce/fee")
        }
        return nil
        
    case TxTransfer:
        // Validate transfer transaction
        if t.Amount == 0 {
            return errors.New("amount must be > 0")
        }
        if err := validateAddress(t.ToAddress); err != nil {
            return fmt.Errorf("invalid toAddress: %w", err)
        }
        if len(t.FromPubKey) != ed25519.PublicKeySize {
            return fmt.Errorf("invalid fromPubKey length: %d", len(t.FromPubKey))
        }
        if len(t.Signature) != ed25519.SignatureSize {
            return fmt.Errorf("invalid signature length: %d", len(t.Signature))
        }
        if t.Nonce == 0 {
            return errors.New("nonce must be > 0")
        }
        if t.ChainID == 0 {
            return errors.New("chainId must be set")
        }
        
        // Verify signature
        h, err := t.signingHashLegacyJSON()
        if err != nil {
            return err
        }
        return t.verifyWithSigningHash(h)
        
    default:
        return fmt.Errorf("unknown transaction type: %q", t.Type)
    }
}
```

#### 2.3.2 Consensus Verification

```go
func (t Transaction) VerifyForConsensus(p ConsensusParams, height uint64) error {
    switch t.Type {
    case TxCoinbase:
        return t.Verify()  // Coinbase verification is the same
        
    case TxTransfer:
        // Re-run structural validation
        if t.Amount == 0 {
            return errors.New("amount must be > 0")
        }
        if err := validateAddress(t.ToAddress); err != nil {
            return fmt.Errorf("invalid toAddress: %w", err)
        }
        if len(t.FromPubKey) != ed25519.PublicKeySize {
            return fmt.Errorf("invalid fromPubKey length: %d", len(t.FromPubKey))
        }
        if len(t.Signature) != ed25519.SignatureSize {
            return fmt.Errorf("invalid signature length: %d", len(t.Signature))
        }
        if t.Nonce == 0 {
            return errors.New("nonce must be > 0")
        }
        if t.ChainID == 0 {
            return errors.New("chainId must be set")
        }
        
        // Verify signature using consensus-selected signing hash
        h, err := txSigningHashForConsensus(t, p, height)
        if err != nil {
            return err
        }
        return t.verifyWithSigningHash(h)
        
    default:
        return fmt.Errorf("unknown transaction type: %q", t.Type)
    }
}
```

#### 2.3.3 Balance and Nonce Verification

```go
func applyBlockToState(p ConsensusParams, state map[string]Account, b *Block) error {
    // ... block validation ...
    
    for i, tx := range b.Transactions {
        switch tx.Type {
        case TxCoinbase:
            // Coinbase: increase recipient balance
            acct := state[tx.ToAddress]
            acct.Balance += tx.Amount
            state[tx.ToAddress] = acct
            
        case TxTransfer:
            fromAddr, err := tx.FromAddress()
            if err != nil {
                return err
            }
            
            from := state[fromAddr]
            
            // Nonce must be sequentially increasing
            if from.Nonce+1 != tx.Nonce {
                return fmt.Errorf("bad nonce for %s: expected %d got %d", 
                    fromAddr, from.Nonce+1, tx.Nonce)
            }
            
            // Balance check (amount + fee)
            totalDebit := tx.Amount + tx.Fee
            if from.Balance < totalDebit {
                return fmt.Errorf("insufficient funds for %s", fromAddr)
            }
            
            // Deduct and update nonce
            from.Balance -= totalDebit
            from.Nonce = tx.Nonce
            state[fromAddr] = from
            
            // Credit recipient
            to := state[tx.ToAddress]
            to.Balance += tx.Amount
            state[tx.ToAddress] = to
        }
    }
    return nil
}
```

### 2.4 Fee Calculation

#### 2.4.1 Minimum Fee

```go
const (
    minFee = uint64(1)  // Minimum transaction fee
)

// Validate fee
if tx.Fee < minFee {
    return nil, fmt.Errorf("fee too low: minFee=%d", minFee)
}
```

#### 2.4.2 Miner Fee Distribution

```go
// Miner fee calculation
func (bc *Blockchain) MineTransfers(transfers []Transaction) (*Block, error) {
    var fees uint64
    for _, tx := range transfers {
        fees += tx.Fee
    }
    
    // Get monetary policy
    policy := bc.consensus.MonetaryPolicy
    
    // Block reward (calculated by height)
    reward := policy.BlockReward(height)
    
    // Miner fee share
    minerFees := policy.MinerFeeAmount(fees)
    
    // Coinbase transaction
    coinbase := Transaction{
        Type:      TxCoinbase,
        ChainID:   bc.ChainID,
        ToAddress: bc.MinerAddress,
        Amount:    reward + minerFees,
        Data:      fmt.Sprintf("block reward + fees (height=%d)", height),
    }
    
    // ...
}
```

### 2.5 Mempool Management

#### 2.5.1 Mempool Structure

```go
type mempoolEntry struct {
    tx       Transaction   // Transaction
    txIDHex  string        // Transaction ID
    received time.Time     // Reception time
}

type Mempool struct {
    mu              sync.Mutex
    maxSize         int
    entries         map[string]mempoolEntry       // txid -> entry
    bySenderNonce   map[string]map[uint64]string  // fromAddr -> nonce -> txid
}
```

#### 2.5.2 Adding Transactions

```go
func (m *Mempool) AddWithTxID(tx Transaction, txid string, p ConsensusParams, height uint64) (string, error) {
    // Calculate transaction ID
    if strings.TrimSpace(txid) == "" {
        if p == (ConsensusParams{}) && height == 0 {
            txid, err = txIDHex(tx)
        } else {
            txid, err = TxIDHexForConsensus(tx, p, height)
        }
        if err != nil {
            return "", err
        }
    }
    
    // Get sender address
    fromAddr, err := tx.FromAddress()
    if err != nil {
        return "", err
    }
    
    m.mu.Lock()
    defer m.mu.Unlock()
    
    // Check for duplicates
    if _, ok := m.entries[txid]; ok {
        return txid, errors.New("duplicate transaction")
    }
    
    // Check nonce conflict
    if existingID, ok := m.bySenderNonce[fromAddr][tx.Nonce]; ok {
        return existingID, errors.New("nonce already in mempool")
    }
    
    // Capacity check (evict lowest fee transaction when full)
    if len(m.entries) >= m.maxSize {
        lowest := m.lowestFeeLocked()
        if lowest == "" {
            return "", errors.New("mempool full")
        }
        m.evictWithDependentsLocked(lowest)
    }
    
    // Add transaction
    m.entries[txid] = mempoolEntry{tx: tx, txIDHex: txid, received: time.Now()}
    m.indexLocked(fromAddr, tx.Nonce, txid)
    
    return txid, nil
}
```

#### 2.5.3 Replace-By-Fee (RBF)

```go
func (m *Mempool) ReplaceByFeeWithTxID(tx Transaction, txid string, p ConsensusParams, height uint64) (string, bool, []string, error) {
    // Calculate transaction ID
    if strings.TrimSpace(txid) == "" {
        txid, err = TxIDHexForConsensus(tx, p, height)
        if err != nil {
            return "", false, nil, err
        }
    }
    
    fromAddr, err := tx.FromAddress()
    if err != nil {
        return "", false, nil, err
    }
    
    m.mu.Lock()
    defer m.mu.Unlock()
    
    // Check if transaction with same nonce exists
    existingID, ok := m.bySenderNonce[fromAddr][tx.Nonce]
    if !ok {
        return "", false, nil, errors.New("no existing nonce to replace")
    }
    
    existing := m.entries[existingID]
    
    // New fee must be strictly greater than old fee
    if tx.Fee <= existing.tx.Fee {
        return "", false, nil, errors.New("replacement fee must be higher")
    }
    
    // Evict old transaction and its dependents (higher nonce transactions)
    evicted := m.evictBySenderNonceLocked(fromAddr, tx.Nonce)
    
    // Insert new transaction
    m.entries[txid] = mempoolEntry{tx: tx, txIDHex: txid, received: time.Now()}
    m.indexLocked(fromAddr, tx.Nonce, txid)
    
    return txid, true, evicted, nil
}

// Evict transactions at specified nonce and higher
func (m *Mempool) evictBySenderNonceLocked(fromAddr string, nonce uint64) []string {
    var evicted []string
    
    // Evict target nonce
    if txid, ok := m.bySenderNonce[fromAddr][nonce]; ok {
        evicted = append(evicted, txid)
        m.removeLocked(txid)
    }
    
    // Evict all higher nonce transactions
    for n, txid := range m.bySenderNonce[fromAddr] {
        if n > nonce {
            evicted = append(evicted, txid)
            m.removeLocked(txid)
        }
    }
    
    return evicted
}
```

#### 2.5.4 Transaction Selection (Block Production)

```go
func (bc *Blockchain) SelectMempoolTxs(mp *Mempool, max int) ([]Transaction, []string, error) {
    if max <= 0 {
        max = 100
    }
    
    // Copy current state
    bc.mu.RLock()
    baseState := make(map[string]Account, len(bc.state))
    for k, v := range bc.state {
        baseState[k] = v
    }
    bc.mu.RUnlock()
    
    // Get transactions sorted by fee descending
    entries := mp.EntriesSortedByFeeDesc()
    var picked []Transaction
    var pickedIDs []string
    
    state := baseState
    nextHeight := bc.LatestBlock().Height + 1
    
    for _, e := range entries {
        if len(picked) >= max {
            break
        }
        
        tx := e.tx
        if tx.Type != TxTransfer {
            continue
        }
        
        // Validate transaction
        if tx.ChainID == 0 {
            tx.ChainID = bc.ChainID
        }
        if err := tx.VerifyForConsensus(bc.consensus, nextHeight); err != nil {
            continue
        }
        
        fromAddr, err := tx.FromAddress()
        if err != nil {
            continue
        }
        
        from := state[fromAddr]
        
        // Nonce must be consecutive
        if from.Nonce+1 != tx.Nonce {
            continue
        }
        
        // Balance check
        totalDebit := tx.Amount + tx.Fee
        if from.Balance < totalDebit {
            continue
        }
        
        // Apply transaction in simulated state (supports consecutive transactions from same sender)
        from.Balance -= totalDebit
        from.Nonce = tx.Nonce
        state[fromAddr] = from
        
        to := state[tx.ToAddress]
        to.Balance += tx.Amount
        state[tx.ToAddress] = to
        
        picked = append(picked, tx)
        pickedIDs = append(pickedIDs, e.txIDHex)
    }
    
    return picked, pickedIDs, nil
}
```

#### 2.5.5 Mempool Queries

```go
// Return all transactions sorted by fee descending
func (m *Mempool) EntriesSortedByFeeDesc() []mempoolEntry {
    entries := m.Snapshot()
    sort.Slice(entries, func(i, j int) bool {
        if entries[i].tx.Fee != entries[j].tx.Fee {
            return entries[i].tx.Fee > entries[j].tx.Fee
        }
        return entries[i].received.Before(entries[j].received)
    })
    return entries
}

// Query pending transactions for sender (sorted by nonce)
func (m *Mempool) PendingForSender(fromAddr string) []mempoolEntry {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    var out []mempoolEntry
    for nonce, txid := range m.bySenderNonce[fromAddr] {
        if e, ok := m.entries[txid]; ok {
            out = append(out, e)
        }
    }
    
    sort.Slice(out, func(i, j int) bool {
        return out[i].tx.Nonce < out[j].tx.Nonce
    })
    
    return out
}

// Query transaction for specified nonce
func (m *Mempool) TxForSenderNonce(fromAddr string, nonce uint64) (mempoolEntry, bool) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    txid, ok := m.bySenderNonce[fromAddr][nonce]
    if !ok {
        return mempoolEntry{}, false
    }
    
    e, ok := m.entries[txid]
    return e, ok
}
```

---

## 3. Block Structure

### 3.1 Block Header

#### 3.1.1 Block Structure Definition

```go
type Block struct {
    Version        uint32        `json:"version"`         // Block version
    Height         uint64        `json:"height"`          // Block height
    TimestampUnix  int64         `json:"timestampUnix"`   // Unix timestamp
    PrevHash       []byte        `json:"prevHash"`        // Previous block hash (Base64)
    Nonce          uint64        `json:"nonce"`           // Proof-of-work nonce
    DifficultyBits uint32        `json:"difficultyBits"`  // Difficulty target
    MinerAddress   string        `json:"minerAddress"`    // Miner address
    Transactions   []Transaction `json:"transactions"`    // Transaction list
    Hash           []byte        `json:"hash"`            // Block hash (Base64)
}
```

#### 3.1.2 Block Header Hash Calculation

**Multi-Version Block Header Encoding**:

```go
func (b *Block) HeaderBytesForConsensus(p ConsensusParams, nonce uint64) ([]byte, error) {
    // Check if binary encoding is enabled
    if p.BinaryEncodingActive(b.Height) {
        return blockHeaderPreimageBinaryV1(b, nonce, p)
    }
    
    switch b.Version {
    case 2:
        // V2 block header (using Merkle root)
        root, err := b.MerkleRootV2ForConsensus(p)
        if err != nil {
            return nil, err
        }
        
        type headerV2 struct {
            Version        uint32 `json:"version"`
            Height         uint64 `json:"height"`
            TimestampUnix  int64  `json:"timestampUnix"`
            PrevHashB64    string `json:"prevHashB64"`
            MerkleRootHex  string `json:"merkleRootHex"`
            DifficultyBits uint32 `json:"difficultyBits"`
            MinerAddress   string `json:"minerAddress"`
            Nonce          uint64 `json:"nonce"`
        }
        
        v := headerV2{
            Version:        b.Version,
            Height:         b.Height,
            TimestampUnix:  b.TimestampUnix,
            PrevHashB64:    base64.StdEncoding.EncodeToString(b.PrevHash),
            MerkleRootHex:  hex.EncodeToString(root),
            DifficultyBits: b.DifficultyBits,
            MinerAddress:   b.MinerAddress,
            Nonce:          nonce,
        }
        
        return json.Marshal(v)
        
    default:
        // V1 block header (using TxRoot)
        root, err := b.TxRootLegacyForConsensus(p)
        if err != nil {
            return nil, err
        }
        
        type headerV1 struct {
            Version        uint32 `json:"version"`
            Height         uint64 `json:"height"`
            TimestampUnix  int64  `json:"timestampUnix"`
            PrevHashB64    string `json:"prevHashB64"`
            TxRootHex      string `json:"txRootHex"`
            DifficultyBits uint32 `json:"difficultyBits"`
            MinerAddress   string `json:"minerAddress"`
            Nonce          uint64 `json:"nonce"`
        }
        
        v := headerV1{
            Version:        b.Version,
            Height:         b.Height,
            TimestampUnix:  b.TimestampUnix,
            PrevHashB64:    base64.StdEncoding.EncodeToString(b.PrevHash),
            TxRootHex:      hex.EncodeToString(root),
            DifficultyBits: b.DifficultyBits,
            MinerAddress:   b.MinerAddress,
            Nonce:          nonce,
        }
        
        return json.Marshal(v)
    }
}
```

#### 3.1.3 Difficulty Adjustment

**ConsensusParams Structure**:

```go
type ConsensusParams struct {
    DifficultyEnable      bool          // Enable difficulty adjustment
    TargetBlockTime       time.Duration // Target block time (default 15 seconds)
    DifficultyWindow      int           // Difficulty adjustment window (default 20 blocks)
    DifficultyMaxStep     uint32        // Maximum difficulty step value
    MinDifficultyBits     uint32        // Minimum difficulty
    MaxDifficultyBits     uint32        // Maximum difficulty (256)
    GenesisDifficultyBits uint32        // Genesis block difficulty
    MedianTimePastWindow  int           // Median time past window
    MaxTimeDrift          int64         // Maximum time drift (seconds)
    MaxBlockSize          uint64        // Maximum block size
    MerkleEnable          bool          // Enable Merkle tree
    MerkleActivationHeight uint64       // Merkle activation height
    BinaryEncodingEnable  bool          // Enable binary encoding
    BinaryEncodingActivationHeight uint64 // Binary encoding activation height
    MonetaryPolicy        MonetaryPolicy // Monetary policy
}
```

**Difficulty Adjustment Algorithm**:

```go
func nextDifficultyBitsFromPath(p ConsensusParams, path []*Block) uint32 {
    if len(path) == 0 {
        return p.GenesisDifficultyBits
    }
    
    parentIdx := len(path) - 1
    parent := path[parentIdx]
    
    // Difficulty adjustment not enabled
    if !p.DifficultyEnable {
        if parent.DifficultyBits == 0 {
            return p.GenesisDifficultyBits
        }
        return clampDifficultyBits(p, parent.DifficultyBits)
    }
    
    // Adaptive window size
    windowSize := p.DifficultyWindow
    if parentIdx < windowSize {
        if parentIdx == 0 {
            // First block: use genesis difficulty
            return clampDifficultyBits(p, p.GenesisDifficultyBits)
        }
        windowSize = parentIdx
    }
    
    olderIdx := parentIdx - windowSize
    older := path[olderIdx]
    
    // Calculate actual time span
    actualSpanSec := parent.TimestampUnix - older.TimestampUnix
    if actualSpanSec <= 0 {
        actualSpanSec = 1
    }
    
    // Calculate expected time span
    targetSec := int64(p.TargetBlockTime / time.Second)
    if targetSec <= 0 {
        targetSec = 1
    }
    expectedSpanSec := int64(windowSize) * targetSec
    
    // Prevent extreme time warp
    if actualSpanSec < expectedSpanSec/4 {
        actualSpanSec = expectedSpanSec / 4
    }
    if actualSpanSec > expectedSpanSec*4 {
        actualSpanSec = expectedSpanSec * 4
    }
    
    // Calculate adjustment ratio
    adjustmentRatio := float64(actualSpanSec) / float64(expectedSpanSec)
    
    next := float64(parent.DifficultyBits)
    
    // Percentage adjustment (50% sensitivity)
    const sensitivity = 0.5
    if adjustmentRatio < 1.0 {
        // Blocks too fast: increase difficulty
        increaseFactor := (1.0 - adjustmentRatio) * sensitivity
        next = next * (1.0 + increaseFactor)
    } else if adjustmentRatio > 1.0 {
        // Blocks too slow: decrease difficulty
        decreaseFactor := (adjustmentRatio - 1.0) * sensitivity
        next = next * (1.0 - decreaseFactor)
    }
    
    // Minimum change of 1 for low difficulties
    if parent.DifficultyBits <= 10 {
        if adjustmentRatio < 1.0 && next <= float64(parent.DifficultyBits) {
            next = float64(parent.DifficultyBits) + 1
        } else if adjustmentRatio > 1.0 && next >= float64(parent.DifficultyBits) {
            if next > 1 {
                next = float64(parent.DifficultyBits) - 1
            }
        }
    }
    
    // Apply maximum step limit
    maxChange := float64(p.DifficultyMaxStep)
    if next > float64(parent.DifficultyBits)+maxChange {
        next = float64(parent.DifficultyBits) + maxChange
    }
    if next < float64(parent.DifficultyBits)-maxChange {
        next = float64(parent.DifficultyBits) - maxChange
    }
    
    if next < 1 {
        next = 1
    }
    
    return clampDifficultyBits(p, uint32(next))
}

// Difficulty range clamping
func clampDifficultyBits(p ConsensusParams, bits uint32) uint32 {
    if bits < 1 {
        bits = 1
    }
    if bits > maxDifficultyBits {
        bits = maxDifficultyBits
    }
    if bits < p.MinDifficultyBits {
        return p.MinDifficultyBits
    }
    if bits > p.MaxDifficultyBits {
        return p.MaxDifficultyBits
    }
    return bits
}
```

### 3.2 Transaction List

#### 3.2.1 Merkle Tree

**Merkle Root Calculation**:

```go
// Calculate Merkle root (Bitcoin style)
func MerkleRoot(leaves [][]byte) ([]byte, error) {
    if len(leaves) == 0 {
        return nil, errors.New("empty leaves")
    }
    
    // Leaf node hash: SHA256(0x00 || leaf)
    level := make([][]byte, 0, len(leaves))
    for _, l := range leaves {
        if len(l) != 32 {
            return nil, errors.New("leaf must be 32 bytes")
        }
        level = append(level, hashLeaf(l))
    }
    
    // Calculate layer by layer
    for len(level) > 1 {
        next := make([][]byte, 0, (len(level)+1)/2)
        for i := 0; i < len(level); i += 2 {
            left := level[i]
            right := left  // Copy last one if odd number
            if i+1 < len(level) {
                right = level[i+1]
            }
            next = append(next, hashNode(left, right))
        }
        level = next
    }
    
    return append([]byte(nil), level[0]...), nil
}

// Leaf node hash: SHA256(0x00 || leaf)
func hashLeaf(leaf []byte) []byte {
    var b [1 + 32]byte
    b[0] = 0x00
    copy(b[1:], leaf)
    sum := sha256.Sum256(b[:])
    return sum[:]
}

// Internal node hash: SHA256(0x01 || left || right)
func hashNode(left, right []byte) []byte {
    var b [1 + 32 + 32]byte
    b[0] = 0x01
    copy(b[1:], left)
    copy(b[33:], right)
    sum := sha256.Sum256(b[:])
    return sum[:]
}
```

**Block Merkle Root**:

```go
// V2 block Merkle root
func (b *Block) MerkleRootV2ForConsensus(p ConsensusParams) ([]byte, error) {
    leaves := make([][]byte, 0, len(b.Transactions))
    
    // Transaction signing hash as leaves
    for _, tx := range b.Transactions {
        th, err := txSigningHashForConsensus(tx, p, b.Height)
        if err != nil {
            return nil, err
        }
        leaves = append(leaves, th)
    }
    
    return MerkleRoot(leaves)
}
```

#### 3.2.2 Merkle Proof

```go
// Generate Merkle proof
func MerkleProof(leaves [][]byte, index int) (branch [][]byte, siblingLeft []bool, root []byte, err error) {
    if len(leaves) == 0 {
        return nil, nil, nil, errors.New("empty leaves")
    }
    if index < 0 || index >= len(leaves) {
        return nil, nil, nil, errors.New("index out of range")
    }
    
    // Build leaf layer
    level := make([][]byte, 0, len(leaves))
    for _, l := range leaves {
        level = append(level, hashLeaf(l))
    }
    
    idx := index
    for len(level) > 1 {
        var sib []byte
        var sibIsLeft bool
        
        if idx%2 == 0 {
            // Sibling on right (or self if copied)
            sibIsLeft = false
            if idx+1 < len(level) {
                sib = level[idx+1]
            } else {
                sib = level[idx]
            }
        } else {
            // Sibling on left
            sibIsLeft = true
            sib = level[idx-1]
        }
        
        branch = append(branch, append([]byte(nil), sib...))
        siblingLeft = append(siblingLeft, sibIsLeft)
        
        // Calculate next layer
        next := make([][]byte, 0, (len(level)+1)/2)
        for i := 0; i < len(level); i += 2 {
            left := level[i]
            right := left
            if i+1 < len(level) {
                right = level[i+1]
            }
            next = append(next, hashNode(left, right))
        }
        level = next
        idx = idx / 2
    }
    
    return branch, siblingLeft, append([]byte(nil), level[0]...), nil
}

// Verify Merkle proof
func VerifyMerkleProof(leaf []byte, index int, branch [][]byte, siblingLeft []bool, expectedRoot []byte) (bool, error) {
    if len(leaf) != 32 {
        return false, errors.New("leaf must be 32 bytes")
    }
    if len(expectedRoot) != 32 {
        return false, errors.New("expected root must be 32 bytes")
    }
    if len(branch) != len(siblingLeft) {
        return false, errors.New("branch/side length mismatch")
    }
    
    cur := hashLeaf(leaf)
    for i := 0; i < len(branch); i++ {
        sib := branch[i]
        if len(sib) != 32 {
            return false, errors.New("branch item must be 32 bytes")
        }
        
        if siblingLeft[i] {
            cur = hashNode(sib, cur)
        } else {
            cur = hashNode(cur, sib)
        }
    }
    
    return string(cur) == string(expectedRoot), nil
}
```

### 3.3 Block Verification

#### 3.3.1 PoW Verification

**Proof-of-Work Verification**:

```go
func (bc *Blockchain) AuditChain() error {
    bc.mu.RLock()
    blocks := append([]*Block(nil), bc.blocks...)
    consensus := bc.consensus
    bc.mu.RUnlock()
    
    if len(blocks) == 0 {
        return errors.New("empty chain")
    }
    
    for i, b := range blocks {
        // Genesis block validation
        if i == 0 {
            if b.Height != 0 || len(b.PrevHash) != 0 {
                return errors.New("invalid genesis header")
            }
            if b.Version != blockVersionForHeight(consensus, 0) {
                return fmt.Errorf("bad block version at %d", b.Height)
            }
        } else {
            // Non-genesis block validation
            prev := blocks[i-1]
            
            // Consecutive height
            if b.Height != prev.Height+1 {
                return fmt.Errorf("bad height at %d", b.Height)
            }
            
            // Previous hash linkage
            if string(b.PrevHash) != string(prev.Hash) {
                return fmt.Errorf("bad prev hash at %d", b.Height)
            }
            
            // Time validation
            if err := validateBlockTime(consensus, blocks, i); err != nil {
                return err
            }
            
            // Difficulty validation
            if consensus.DifficultyEnable {
                expected := expectedDifficultyBitsForBlockIndex(consensus, blocks, i)
                if b.DifficultyBits != expected {
                    return fmt.Errorf("bad difficulty at %d: expected %d got %d", 
                        b.Height, expected, b.DifficultyBits)
                }
            }
            
            // Version validation
            if b.Version != blockVersionForHeight(consensus, b.Height) {
                return fmt.Errorf("bad block version at %d", b.Height)
            }
        }
        
        // Difficulty range validation
        if b.DifficultyBits == 0 || b.DifficultyBits > maxDifficultyBits {
            return fmt.Errorf("difficultyBits out of range at %d: %d", b.Height, b.DifficultyBits)
        }
        
        // PoW validation
        ok, err := NewProofOfWork(consensus, b).Validate()
        if err != nil {
            return err
        }
        if !ok {
            return fmt.Errorf("invalid pow at height %d", b.Height)
        }
        
        // Transaction validation
        for _, tx := range b.Transactions {
            if tx.ChainID == 0 {
                return fmt.Errorf("missing chainId at height %d", b.Height)
            }
            if err := tx.VerifyForConsensus(consensus, b.Height); err != nil {
                return fmt.Errorf("invalid tx at height %d: %w", b.Height, err)
            }
        }
    }
    
    // State reproducibility validation
    return bc.recomputeState()
}
```

#### 3.3.2 Time Validation

```go
func validateBlockTime(p ConsensusParams, blocks []*Block, idx int) error {
    if idx <= 0 || idx >= len(blocks) {
        return nil
    }
    
    current := blocks[idx]
    prev := blocks[idx-1]
    
    // Time must be greater than previous block time
    if current.TimestampUnix <= prev.TimestampUnix {
        return fmt.Errorf("block time must be greater than previous at %d", idx)
    }
    
    // Median time past validation
    if p.MedianTimePastWindow > 0 && idx >= p.MedianTimePastWindow {
        times := make([]int64, p.MedianTimePastWindow)
        for i := 0; i < p.MedianTimePastWindow; i++ {
            times[i] = blocks[idx-1-i].TimestampUnix
        }
        sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
        medianTime := times[p.MedianTimePastWindow/2]
        
        if current.TimestampUnix < medianTime {
            return fmt.Errorf("block time less than median time past at %d", idx)
        }
    }
    
    // Maximum time drift validation
    maxFuture := time.Now().Unix() + p.MaxTimeDrift
    if current.TimestampUnix > maxFuture {
        return fmt.Errorf("block time too far in future at %d", idx)
    }
    
    return nil
}
```

#### 3.3.3 Coinbase Verification

```go
func applyBlockToState(p ConsensusParams, state map[string]Account, b *Block) error {
    // ...
    
    // Non-genesis block: validate coinbase amount
    if b.Height > 0 {
        if err := validateAddress(b.MinerAddress); err != nil {
            return fmt.Errorf("invalid minerAddress: %w", err)
        }
        
        // Calculate total fees
        var fees uint64
        for _, tx := range b.Transactions[1:] {
            if tx.Type != TxTransfer {
                continue
            }
            fees += tx.Fee
        }
        
        cb := b.Transactions[0]
        
        // Coinbase recipient address must match miner address
        if cb.ToAddress != b.MinerAddress {
            return errors.New("coinbase toAddress must match minerAddress")
        }
        
        // Validate coinbase amount (block reward + miner fee share)
        policy := p.MonetaryPolicy
        expected := policy.BlockReward(b.Height) + policy.MinerFeeAmount(fees)
        
        if cb.Amount != expected {
            return fmt.Errorf("bad coinbase amount: expected %d got %d", expected, cb.Amount)
        }
    }
    
    // ...
}
```

### 3.4 Block Index

#### 3.4.1 Transaction Index

```go
type TxLocation struct {
    Height       uint64 `json:"height"`         // Block height
    BlockHashHex string `json:"blockHashHex"`   // Block hash (hexadecimal)
    Index        int    `json:"index"`          // Transaction index in block
}

// Index transactions
func (bc *Blockchain) indexTxsForBlockLocked(b *Block) {
    if bc.txIndex == nil {
        bc.txIndex = map[string]TxLocation{}
    }
    
    hashHex := hex.EncodeToString(b.Hash)
    
    for i, tx := range b.Transactions {
        // Only index transfer transactions (coinbase txid may conflict)
        if tx.Type != TxTransfer {
            continue
        }
        
        txid, err := TxIDHexForConsensus(tx, bc.consensus, b.Height)
        if err != nil {
            continue
        }
        
        bc.txIndex[txid] = TxLocation{
            Height:       b.Height,
            BlockHashHex: hashHex,
            Index:        i,
        }
    }
}

// Query transaction
func (bc *Blockchain) TxByID(txid string) (Transaction, TxLocation, bool) {
    bc.mu.RLock()
    defer bc.mu.RUnlock()
    
    loc, ok := bc.txIndex[txid]
    if !ok {
        return Transaction{}, TxLocation{}, false
    }
    
    if loc.Height >= uint64(len(bc.blocks)) || loc.Index < 0 {
        return Transaction{}, TxLocation{}, false
    }
    
    b := bc.blocks[int(loc.Height)]
    if loc.Index >= len(b.Transactions) {
        return Transaction{}, TxLocation{}, false
    }
    
    if hex.EncodeToString(b.Hash) != loc.BlockHashHex {
        return Transaction{}, TxLocation{}, false
    }
    
    return b.Transactions[loc.Index], loc, true
}
```

#### 3.4.2 Address Index

```go
type AddressTxEntry struct {
    TxID      string     `json:"txId"`
    Location  TxLocation `json:"location"`
    FromAddr  string     `json:"fromAddr"`
    ToAddress string     `json:"toAddress"`
    Amount    uint64     `json:"amount"`
    Fee       uint64     `json:"fee"`
    Nonce     uint64     `json:"nonce"`
}

// Index address transactions
func (bc *Blockchain) indexAddressTxsForBlockLocked(b *Block) {
    if bc.addressIndex == nil {
        bc.addressIndex = map[string][]AddressTxEntry{}
    }
    
    hashHex := hex.EncodeToString(b.Hash)
    
    for i, tx := range b.Transactions {
        if tx.Type != TxTransfer {
            continue
        }
        
        txid, err := TxIDHexForConsensus(tx, bc.consensus, b.Height)
        if err != nil {
            continue
        }
        
        from, err := tx.FromAddress()
        if err != nil {
            continue
        }
        
        entry := AddressTxEntry{
            TxID: txid,
            Location: TxLocation{
                Height:       b.Height,
                BlockHashHex: hashHex,
                Index:        i,
            },
            FromAddr:  from,
            ToAddress: tx.ToAddress,
            Amount:    tx.Amount,
            Fee:       tx.Fee,
            Nonce:     tx.Nonce,
        }
        
        // Sender index
        bc.addressIndex[from] = append(bc.addressIndex[from], entry)
        
        // Recipient index (if different)
        if tx.ToAddress != from {
            bc.addressIndex[tx.ToAddress] = append(bc.addressIndex[tx.ToAddress], entry)
        }
    }
}

// Query address transaction history (newest first)
func (bc *Blockchain) AddressTxs(address string, limit int, cursor int) ([]AddressTxEntry, int, bool) {
    bc.mu.RLock()
    defer bc.mu.RUnlock()
    
    if bc.addressIndex == nil {
        return nil, 0, false
    }
    
    all := bc.addressIndex[address]
    if len(all) == 0 {
        return []AddressTxEntry{}, 0, false
    }
    
    if limit <= 0 {
        limit = 50
    }
    if limit > 200 {
        limit = 200
    }
    if cursor < 0 {
        cursor = 0
    }
    
    // Start from newest (end of array)
    start := len(all) - 1 - cursor
    if start < 0 {
        return []AddressTxEntry{}, cursor, false
    }
    
    out := make([]AddressTxEntry, 0, limit)
    i := start
    for i >= 0 && len(out) < limit {
        out = append(out, all[i])
        i--
    }
    
    nextCursor := cursor + len(out)
    more := (len(all) - 1 - nextCursor) >= 0
    
    return out, nextCursor, more
}
```

#### 3.4.3 Height Index

```go
type Blockchain struct {
    // ...
    blocks         []*Block              // Canonical chain (sorted by height)
    blocksByHash   map[string]*Block     // Hash -> block
    bestTipHash    string                // Best tip hash
    canonicalWork  *big.Int              // Canonical chain cumulative work
}

// Query block by height
func (bc *Blockchain) BlockByHeight(height uint64) (*Block, bool) {
    bc.mu.RLock()
    defer bc.mu.RUnlock()
    
    if height >= uint64(len(bc.blocks)) {
        return nil, false
    }
    
    return bc.blocks[int(height)], true
}

// Query block by hash
func (bc *Blockchain) BlockByHash(hashHex string) (*Block, bool) {
    bc.mu.RLock()
    defer bc.mu.RUnlock()
    
    block, ok := bc.blocksByHash[hashHex]
    if !ok {
        return nil, false
    }
    
    return block, true
}

// Get latest block
func (bc *Blockchain) LatestBlock() *Block {
    bc.mu.RLock()
    defer bc.mu.RUnlock()
    return bc.blocks[len(bc.blocks)-1]
}
```

---

## 4. Network Protocol

### 4.1 P2P Communication

#### 4.1.1 Message Format

**P2P Envelope Structure**:

```go
type p2pEnvelope struct {
    Type    string          `json:"type"`     // Message type
    Payload json.RawMessage `json:"payload"`  // Message payload
}
```

**Message Types**:

| Type | Description | Direction |
|------|-------------|-----------|
| `hello` | Handshake message | Bidirectional |
| `chain_info_req` | Chain info query request | Client→Server |
| `chain_info` | Chain info response | Server→Client |
| `headers_from_req` | Block header query request | Client→Server |
| `headers` | Block header response | Server→Client |
| `block_by_hash_req` | Block query request | Client→Server |
| `block` | Block response | Server→Client |
| `tx_req` | Transaction query request | Client→Server |
| `tx_ack` | Transaction acknowledgment | Server→Client |
| `tx_broadcast` | Transaction broadcast | Client→Server |
| `tx_broadcast_ack` | Transaction broadcast acknowledgment | Server→Client |
| `block_broadcast` | Block broadcast | Client→Server |
| `block_broadcast_ack` | Block broadcast acknowledgment | Server→Client |
| `block_req` | Block request | Client→Server |
| `getaddr` | Get node list | Client→Server |
| `addr` | Node list response | Server→Client |
| `addr_ack` | Node list acknowledgment | Server→Client |
| `error` | Error message | Bidirectional |

#### 4.1.2 Encoding/Decoding

**Message Encoding (TLV Format)**:

```go
// 4-byte length + JSON payload
func p2pWriteJSON(w io.Writer, v any) error {
    b, err := json.Marshal(v)
    if err != nil {
        return err
    }
    
    // Big-endian 4-byte length
    var lenBuf [4]byte
    binary.BigEndian.PutUint32(lenBuf[:], uint32(len(b)))
    
    if _, err := w.Write(lenBuf[:]); err != nil {
        return err
    }
    
    _, err = w.Write(b)
    return err
}
```

**Message Decoding**:

```go
func p2pReadJSON(r io.Reader, maxBytes int) ([]byte, error) {
    // Read 4-byte length
    var lenBuf [4]byte
    if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
        return nil, err
    }
    
    n := int(binary.BigEndian.Uint32(lenBuf[:]))
    if n <= 0 {
        return nil, io.ErrUnexpectedEOF
    }
    
    // Size limit
    if maxBytes > 0 && n > maxBytes {
        _, _ = io.CopyN(io.Discard, r, int64(n))
        return nil, ErrP2PTooLarge
    }
    
    // Read payload
    b := make([]byte, n)
    if _, err := io.ReadFull(r, b); err != nil {
        return nil, err
    }
    
    return b, nil
}
```

#### 4.1.3 Handshake Protocol

**Hello Message**:

```go
type p2pHello struct {
    Protocol  int    `json:"protocol"`   // Protocol version (currently 1)
    ChainID   uint64 `json:"chainId"`    // Chain ID
    RulesHash string `json:"rulesHash"`  // Consensus rules hash
    NodeID    string `json:"nodeId"`     // Node ID
    TimeUnix  int64  `json:"timeUnix"`   // Timestamp
}

func newP2PHello(chainID uint64, rulesHash string, nodeID string) p2pHello {
    return p2pHello{
        Protocol:  1,
        ChainID:   chainID,
        RulesHash: rulesHash,
        NodeID:    nodeID,
        TimeUnix:  time.Now().Unix(),
    }
}
```

**Handshake Flow**:

```go
func (s *P2PServer) handleConn(c net.Conn) error {
    defer c.Close()
    
    // Set timeout
    _ = c.SetDeadline(time.Now().Add(15 * time.Second))
    
    // 1. Receive Hello
    raw, err := p2pReadJSON(c, 1<<20)
    if err != nil {
        return err
    }
    
    var env p2pEnvelope
    if err := json.Unmarshal(raw, &env); err != nil {
        return err
    }
    
    if env.Type != "hello" {
        return errors.New("expected hello")
    }
    
    var hello p2pHello
    if err := json.Unmarshal(env.Payload, &hello); err != nil {
        return err
    }
    
    // 2. Validate protocol and chain ID
    if hello.Protocol != 1 || hello.ChainID != s.bc.ChainID {
        _ = p2pWriteJSON(c, p2pEnvelope{
            Type: "error", 
            Payload: mustJSON(map[string]any{"error": "wrong_chain_or_protocol"}),
        })
        return errors.New("wrong chain/protocol")
    }
    
    // 3. Validate consensus rules hash
    if hello.RulesHash != s.bc.RulesHashHex() {
        _ = p2pWriteJSON(c, p2pEnvelope{
            Type: "error",
            Payload: mustJSON(map[string]any{"error": "rules_hash_mismatch"}),
        })
        return errors.New("rules hash mismatch")
    }
    
    // 4. Reply Hello
    _ = p2pWriteJSON(c, p2pEnvelope{
        Type: "hello",
        Payload: mustJSON(newP2PHello(s.bc.ChainID, s.bc.RulesHashHex(), s.nodeID)),
    })
    
    // 5. Process requests
    raw, err = p2pReadJSON(c, s.maxMsgSize)
    // ...
}
```

### 4.2 Node Discovery

#### 4.2.1 Node Scoring

**PeerScore Structure**:

```go
type PeerScore struct {
    Peer           string      // Peer address
    Score          float64     // Score (0-100)
    SuccessCount   int         // Success count
    FailureCount   int         // Failure count
    TotalLatencyMs int64       // Total latency (milliseconds)
    LastSeen       time.Time   // Last active time
    FirstSeen      time.Time   // First discovery time
}
```

**Scoring Algorithm**:

```go
type PeerScorer struct {
    mu       sync.RWMutex
    peers    map[string]*PeerScore
    maxPeers int
}

func (ps *PeerScorer) calculateScore(p *PeerScore) float64 {
    total := p.SuccessCount + p.FailureCount
    if total == 0 {
        return 50.0
    }
    
    // Success rate
    successRate := float64(p.SuccessCount) / float64(total)
    
    // Average latency
    var avgLatency float64 = 1000
    if p.SuccessCount > 0 {
        avgLatency = float64(p.TotalLatencyMs) / float64(p.SuccessCount)
    }
    
    // Latency factor
    latencyFactor := 1.0
    if avgLatency < 100 {
        latencyFactor = 1.5    // <100ms: excellent
    } else if avgLatency < 500 {
        latencyFactor = 1.2    // <500ms: good
    } else if avgLatency > 2000 {
        latencyFactor = 0.5    // >2s: poor
    } else if avgLatency > 5000 {
        latencyFactor = 0.2    // >5s: very poor
    }
    
    // Calculate score
    score := successRate * 100 * latencyFactor
    
    if score > 100 {
        score = 100
    }
    if score < 0 {
        score = 0
    }
    
    return score
}
```

**Record Success/Failure**:

```go
func (ps *PeerScorer) RecordSuccess(peer string, latencyMs int64) {
    ps.mu.Lock()
    defer ps.mu.Unlock()
    
    now := time.Now()
    if p, ok := ps.peers[peer]; ok {
        // Update existing peer
        p.SuccessCount++
        p.TotalLatencyMs += latencyMs
        p.LastSeen = now
        p.Score = ps.calculateScore(p)
    } else {
        // Add new peer
        ps.peers[peer] = &PeerScore{
            Peer:           peer,
            Score:          50.0,
            SuccessCount:   1,
            FailureCount:   0,
            TotalLatencyMs: latencyMs,
            LastSeen:       now,
            FirstSeen:      now,
        }
        ps.evictIfNeeded()
    }
}

func (ps *PeerScorer) RecordFailure(peer string) {
    ps.mu.Lock()
    defer ps.mu.Unlock()
    
    now := time.Now()
    if p, ok := ps.peers[peer]; ok {
        p.FailureCount++
        p.LastSeen = now
        p.Score = ps.calculateScore(p)
    } else {
        ps.peers[peer] = &PeerScore{
            Peer:           peer,
            Score:          25.0,
            SuccessCount:   0,
            FailureCount:   1,
            TotalLatencyMs: 0,
            LastSeen:       now,
            FirstSeen:      now,
        }
        ps.evictIfNeeded()
    }
}
```

**Get Top Peers**:

```go
func (ps *PeerScorer) GetTopPeers(n int) []string {
    ps.mu.RLock()
    defer ps.mu.RUnlock()
    
    type scoredPeer struct {
        peer  string
        score float64
    }
    
    var scored []scoredPeer
    for peer, p := range ps.peers {
        scored = append(scored, scoredPeer{peer: peer, score: p.Score})
    }
    
    // Sort by score descending
    for i := 0; i < len(scored)-1; i++ {
        for j := i + 1; j < len(scored); j++ {
            if scored[j].score > scored[i].score {
                scored[i], scored[j] = scored[j], scored[i]
            }
        }
    }
    
    if n > len(scored) {
        n = len(scored)
    }
    
    result := make([]string, n)
    for i := 0; i < n; i++ {
        result[i] = scored[i].peer
    }
    return result
}
```

#### 4.2.2 Node Management

**PeerManager Structure**:

```go
type PeerManager struct {
    peers []string
    
    client *http.Client
    
    maxAncestorDepth int
}

func NewPeerManager(peers []string) *PeerManager {
    return &PeerManager{
        peers: peers,
        client: &http.Client{
            Timeout: 10 * time.Second,
        },
        maxAncestorDepth: 256,
    }
}

// Add peer
func (pm *PeerManager) AddPeer(addr string) {
    if addr == "" {
        return
    }
    
    // Deduplicate
    for _, p := range pm.peers {
        if p == addr {
            return
        }
    }
    
    pm.peers = append(pm.peers, addr)
}

// Parse peer list (environment variable format)
func ParsePeersEnv(peersEnv string) []string {
    var peers []string
    for _, raw := range strings.Split(peersEnv, ",") {
        raw = strings.TrimSpace(raw)
        if raw == "" {
            continue
        }
        raw = strings.TrimRight(raw, "/")
        if _, err := url.Parse(raw); err != nil {
            continue
        }
        peers = append(peers, raw)
    }
    return peers
}
```

### 4.3 Data Synchronization

#### 4.3.1 Block Synchronization

**Fetch Chain Info**:

```go
type chainInfo struct {
    ChainID     uint64 `json:"chainId"`
    Height      uint64 `json:"height"`
    LatestHash  string `json:"latestHash"`
    RulesHash   string `json:"rulesHash"`
    GenesisHash string `json:"genesisHash"`
}

func (pm *PeerManager) FetchChainInfo(ctx context.Context, peer string) (*chainInfo, error) {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, peer+"/chain/info", nil)
    if err != nil {
        return nil, err
    }
    
    resp, err := pm.client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("chain/info status: %s", resp.Status)
    }
    
    var v chainInfo
    if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&v); err != nil {
        return nil, err
    }
    
    return &v, nil
}
```

**Fetch Block Headers**:

```go
func (pm *PeerManager) FetchHeadersFrom(ctx context.Context, peer string, fromHeight uint64, count int) ([]BlockHeader, error) {
    u := fmt.Sprintf("%s/headers/from/%d?count=%d", peer, fromHeight, count)
    
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
    if err != nil {
        return nil, err
    }
    
    resp, err := pm.client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("headers status: %s", resp.Status)
    }
    
    var headers []BlockHeader
    if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&headers); err != nil {
        return nil, err
    }
    
    return headers, nil
}
```

**Fetch Block**:

```go
func (pm *PeerManager) FetchBlockByHash(ctx context.Context, peer string, hashHex string) (*Block, error) {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, peer+"/blocks/hash/"+hashHex, nil)
    if err != nil {
        return nil, err
    }
    
    resp, err := pm.client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode == http.StatusNotFound {
        return nil, errors.New("not found")
    }
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("block status: %s", resp.Status)
    }
    
    var b Block
    if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&b); err != nil {
        return nil, err
    }
    
    return &b, nil
}
```

**Ensure Ancestor Blocks**:

```go
func (pm *PeerManager) EnsureAncestors(ctx context.Context, bc *Blockchain, missingHashHex string) error {
    need := missingHashHex
    visited := map[string]struct{}{}
    
    for depth := 0; depth < pm.maxAncestorDepth; depth++ {
        // Already have locally
        if _, ok := bc.BlockByHash(need); ok {
            return nil
        }
        
        // Prevent cycles
        if _, ok := visited[need]; ok {
            return errors.New("ancestor fetch cycle")
        }
        visited[need] = struct{}{}
        
        // Fetch block
        b, _, err := pm.FetchAnyBlockByHash(ctx, need)
        if err != nil {
            return err
        }
        
        // Recursively ensure parent block
        parentHex := fmt.Sprintf("%x", b.PrevHash)
        if len(b.PrevHash) != 0 {
            if _, ok := bc.BlockByHash(parentHex); !ok {
                if err := pm.EnsureAncestors(ctx, bc, parentHex); err != nil {
                    return err
                }
            }
        }
        
        // Add block
        _, err = bc.AddBlock(b)
        if err == nil {
            return nil
        }
        if errors.Is(err, ErrUnknownParent) {
            continue
        }
        return err
    }
    
    return errors.New("max ancestor depth exceeded")
}
```

#### 4.3.2 Transaction Broadcast

**Broadcast Transaction**:

```go
const relayHopsHeader = "X-NogoChain-Relay-Hops"

func (pm *PeerManager) BroadcastTransaction(ctx context.Context, tx Transaction, hops int) {
    if pm == nil || len(pm.peers) == 0 || hops <= 0 {
        return
    }
    
    b, err := json.Marshal(tx)
    if err != nil {
        return
    }
    
    for _, peer := range pm.peers {
        peer := peer
        go func() {
            req, err := http.NewRequestWithContext(ctx, http.MethodPost, peer+"/tx", bytes.NewReader(b))
            if err != nil {
                return
            }
            
            req.Header.Set("Content-Type", "application/json")
            req.Header.Set(relayHopsHeader, strconv.Itoa(hops))
            
            resp, err := pm.client.Do(req)
            if err != nil {
                return
            }
            _ = resp.Body.Close()
        }()
    }
}
```

**P2P Transaction Broadcast**:

```go
func (s *P2PServer) handleTransactionBroadcast(c net.Conn, payload json.RawMessage) error {
    var broadcast p2pTransactionBroadcast
    if err := json.Unmarshal(payload, &broadcast); err != nil {
        return err
    }
    
    var tx Transaction
    if err := json.Unmarshal([]byte(broadcast.TxHex), &tx); err != nil {
        return err
    }
    
    txid, err := TxIDHex(tx)
    if err != nil {
        return err
    }
    
    // Add to mempool
    if s.mp != nil {
        _, _ = s.mp.Add(tx)
    }
    
    return p2pWriteJSON(c, p2pEnvelope{
        Type: "tx_broadcast_ack",
        Payload: mustJSON(map[string]any{"txid": txid}),
    })
}
```

### 4.4 Security Mechanisms

#### 4.4.1 Connection Management

```go
type P2PServer struct {
    bc *Blockchain
    pm PeerAPI
    mp *Mempool
    
    listenAddr string
    nodeID     string
    
    maxConns    int       // Maximum connections
    maxMsgSize  int       // Maximum message size
    sem         chan struct{}  // Connection semaphore
    blockRecvCB func(*Block)
}

func NewP2PServer(bc *Blockchain, pm PeerAPI, mp *Mempool, listenAddr string, nodeID string) *P2PServer {
    if strings.TrimSpace(listenAddr) == "" {
        listenAddr = ":9090"
    }
    if strings.TrimSpace(nodeID) == "" {
        nodeID = bc.MinerAddress
    }
    
    s := &P2PServer{
        bc:         bc,
        pm:         pm,
        mp:         mp,
        listenAddr: listenAddr,
        nodeID:     nodeID,
        maxConns:   envInt("P2P_MAX_CONNECTIONS", 200),
        maxMsgSize: envInt("P2P_MAX_MESSAGE_BYTES", 4<<20),
    }
    
    if s.maxConns <= 0 {
        s.maxConns = 200
    }
    if s.maxMsgSize <= 0 {
        s.maxMsgSize = 4 << 20
    }
    
    // Semaphore to limit concurrent connections
    s.sem = make(chan struct{}, s.maxConns)
    
    return s
}
```

#### 4.4.2 Message Size Limit

```go
func (s *P2PServer) handleConn(c net.Conn) error {
    defer c.Close()
    
    // Set read timeout
    _ = c.SetDeadline(time.Now().Add(15 * time.Second))
    
    // Read message (with size limit)
    raw, err := p2pReadJSON(c, s.maxMsgSize)
    if err != nil {
        return err
    }
    
    // ...
}
```

#### 4.4.3 Consensus Rules Validation

```go
// Validate consensus rules during handshake
if strings.TrimSpace(hello.RulesHash) == "" || hello.RulesHash != s.bc.RulesHashHex() {
    _ = p2pWriteJSON(c, p2pEnvelope{
        Type: "error",
        Payload: mustJSON(map[string]any{"error": "rules_hash_mismatch"}),
    })
    return errors.New("rules hash mismatch")
}
```

---

## 5. Smart Contracts

### 5.1 VM Architecture

#### 5.1.1 Virtual Machine Structure

```go
type VM struct {
    stack     []interface{}  // Operand stack
    callStack []int          // Call stack
    pc        int            // Program counter
    code      []byte         // Bytecode
    storage   map[string][]byte  // Storage space
    gasLimit  int64          // Gas limit
    gasUsed   int64          // Gas used
}

type VMResult struct {
    Success bool
    Data    []byte
    GasUsed int64
    Error   string
}
```

#### 5.1.2 Opcode Definition

```go
const (
    // Control flow
    OP_NOP           = 0x00  // No operation
    OP_JUMP          = 0xE1  // Jump
    OP_JUMPI         = 0xE2  // Conditional jump
    OP_RETURN        = 0xF0  // Return
    OP_CALL          = 0xE0  // Call
    
    // Stack operations
    OP_PUSH1         = 0x01  // Push 1 byte
    OP_PUSH2         = 0x02  // Push 2 bytes
    OP_PUSH32        = 0x20  // Push 32 bytes
    OP_DUP           = 0x30  // Duplicate stack top
    OP_SWAP          = 0x31  // Swap top two stack items
    OP_DROP          = 0x32  // Drop stack top
    OP_TOALTSTACK    = 0x33  // Move to alternate stack
    OP_FROMALTSTACK  = 0x34  // Move from alternate stack
    
    // Arithmetic operations
    OP_ADD           = 0x40  // Addition
    OP_SUB           = 0x41  // Subtraction
    OP_MUL           = 0x42  // Multiplication
    OP_DIV           = 0x43  // Division
    OP_MOD           = 0x44  // Modulo
    
    // Hash operations
    OP_SHA256        = 0x50  // SHA256
    
    // Comparison operations
    OP_EQUAL         = 0x60  // Equal
    OP_VERIFY        = 0x61  // Verify
    
    // Signature verification
    OP_CHECKSIG      = 0x70  // Verify signature
    OP_CHECKMULTISIG = 0x71  // Verify multisig
)
```

#### 5.1.3 Execution Engine

```go
func (vm *VM) Run() VMResult {
    for vm.pc < len(vm.code) {
        // Gas check
        if vm.gasUsed > vm.gasLimit {
            return VMResult{
                Success: false,
                Error: "gas limit exceeded",
                GasUsed: vm.gasUsed,
            }
        }
        
        op := vm.code[vm.pc]
        vm.pc++
        
        switch op {
        case OP_NOP:
            vm.gasUsed++
            
        case OP_PUSH1:
            if vm.pc >= len(vm.code) {
                return VMResult{Success: false, Error: "unexpected end of code"}
            }
            vm.stack = append(vm.stack, int64(vm.code[vm.pc]))
            vm.pc++
            vm.gasUsed += 3
            
        case OP_ADD:
            if len(vm.stack) < 2 {
                return VMResult{Success: false, Error: "stack underflow"}
            }
            a := vm.stack[len(vm.stack)-2].(int64)
            b := vm.stack[len(vm.stack)-1].(int64)
            vm.stack = vm.stack[:len(vm.stack)-2]
            vm.stack = append(vm.stack, a+b)
            vm.gasUsed += 3
            
        case OP_SUB:
            if len(vm.stack) < 2 {
                return VMResult{Success: false, Error: "stack underflow"}
            }
            a := vm.stack[len(vm.stack)-2].(int64)
            b := vm.stack[len(vm.stack)-1].(int64)
            vm.stack = vm.stack[:len(vm.stack)-2]
            vm.stack = append(vm.stack, a-b)
            vm.gasUsed += 3
            
        case OP_MUL:
            if len(vm.stack) < 2 {
                return VMResult{Success: false, Error: "stack underflow"}
            }
            a := vm.stack[len(vm.stack)-2].(int64)
            b := vm.stack[len(vm.stack)-1].(int64)
            vm.stack = vm.stack[:len(vm.stack)-2]
            vm.stack = append(vm.stack, a*b)
            vm.gasUsed += 5
            
        case OP_DIV:
            if len(vm.stack) < 2 {
                return VMResult{Success: false, Error: "stack underflow"}
            }
            a := vm.stack[len(vm.stack)-2].(int64)
            b := vm.stack[len(vm.stack)-1].(int64)
            if b == 0 {
                return VMResult{Success: false, Error: "division by zero"}
            }
            vm.stack = vm.stack[:len(vm.stack)-2]
            vm.stack = append(vm.stack, a/b)
            vm.gasUsed += 5
            
        case OP_EQUAL:
            if len(vm.stack) < 2 {
                return VMResult{Success: false, Error: "stack underflow"}
            }
            a := vm.stack[len(vm.stack)-2]
            b := vm.stack[len(vm.stack)-1]
            vm.stack = vm.stack[:len(vm.stack)-2]
            vm.stack = append(vm.stack, a == b)
            vm.gasUsed += 3
            
        case OP_VERIFY:
            if len(vm.stack) < 1 {
                return VMResult{Success: false, Error: "stack underflow"}
            }
            if !vm.stack[len(vm.stack)-1].(bool) {
                return VMResult{Success: false, Error: "verify failed"}
            }
            vm.stack = vm.stack[:len(vm.stack)-1]
            vm.gasUsed += 5
            
        case OP_JUMP:
            if len(vm.stack) < 1 {
                return VMResult{Success: false, Error: "stack underflow"}
            }
            target := int(vm.stack[len(vm.stack)-1].(int64))
            vm.stack = vm.stack[:len(vm.stack)-1]
            if target < 0 || target >= len(vm.code) {
                return VMResult{Success: false, Error: "invalid jump target"}
            }
            vm.pc = target
            vm.gasUsed += 5
            
        case OP_RETURN:
            var result []byte
            if len(vm.stack) > 0 {
                switch v := vm.stack[len(vm.stack)-1].(type) {
                case int64:
                    result = []byte(fmt.Sprintf("%d", v))
                case bool:
                    if v {
                        result = []byte("true")
                    } else {
                        result = []byte("false")
                    }
                case []byte:
                    result = v
                }
            }
            return VMResult{Success: true, Data: result, GasUsed: vm.gasUsed}
            
        default:
            return VMResult{Success: false, Error: fmt.Sprintf("unknown opcode: %x", op)}
        }
    }
    
    return VMResult{Success: true, GasUsed: vm.gasUsed}
}
```

### 5.2 Gas Mechanism

#### 5.2.1 Gas Calculation

```go
// Opcode gas costs
const (
    OP_NOP_GAS           = 1
    OP_PUSH1_GAS         = 3
    OP_PUSH2_GAS         = 5
    OP_DUP_GAS           = 2
    OP_DROP_GAS          = 2
    OP_SWAP_GAS          = 3
    OP_ADD_GAS           = 3
    OP_SUB_GAS           = 3
    OP_MUL_GAS           = 5
    OP_DIV_GAS           = 5
    OP_EQUAL_GAS         = 3
    OP_VERIFY_GAS        = 5
    OP_JUMP_GAS          = 5
)

// Set gas limit when creating VM
func NewVM(code []byte, storage map[string][]byte, gasLimit int64) *VM {
    return &VM{
        stack:     make([]interface{}, 0),
        callStack: make([]int, 0),
        pc:        0,
        code:      code,
        storage:   storage,
        gasLimit:  gasLimit,
        gasUsed:   0,
    }
}

// Check gas during execution
func (vm *VM) Run() VMResult {
    for vm.pc < len(vm.code) {
        if vm.gasUsed > vm.gasLimit {
            return VMResult{
                Success: false,
                Error: "gas limit exceeded",
                GasUsed: vm.gasUsed,
            }
        }
        // ...
    }
}
```

### 5.3 Contract Interface

#### 5.3.1 Smart Contract Structure

```go
type SmartContract struct {
    Code    []byte            // Bytecode
    Storage map[string][]byte // Storage space
}

func NewSmartContract(codeHex string) (*SmartContract, error) {
    code, err := hex.DecodeString(codeHex)
    if err != nil {
        return nil, err
    }
    
    return &SmartContract{
        Code:    code,
        Storage: make(map[string][]byte),
    }, nil
}

// Deploy contract
func (sc *SmartContract) Deploy(gasLimit int64) VMResult {
    vm := NewVM(sc.Code, sc.Storage, gasLimit)
    return vm.Run()
}

// Call contract
func (sc *SmartContract) Call(method string, params []interface{}, gasLimit int64) VMResult {
    vm := NewVM(sc.Code, sc.Storage, gasLimit)
    return vm.Run()
}

// Storage operations
func (sc *SmartContract) GetStorage(key string) []byte {
    return sc.Storage[key]
}

func (sc *SmartContract) SetStorage(key string, value []byte) {
    sc.Storage[key] = value
}
```

#### 5.3.2 Contract Data Parsing

```go
func ParseContractData(data string) (string, []interface{}, error) {
    // Format: method:param1:param2:...
    parts := strings.Split(data, ":")
    if len(parts) < 2 {
        return "", nil, fmt.Errorf("invalid contract data format")
    }
    
    method := parts[0]
    var params []interface{}
    
    if len(parts) > 1 {
        for _, p := range parts[1:] {
            params = append(params, p)
        }
    }
    
    return method, params, nil
}
```

### 5.4 Contract Security

#### 5.4.1 Token Contract

```go
type TokenContract struct {
    *SmartContract
    Name        string             // Token name
    Symbol      string             // Token symbol
    Decimals    uint8              // Decimal places
    TotalSupply uint64             // Total supply
    Balances    map[string]uint64  // Balance mapping
}

func NewTokenContract(name, symbol string, decimals uint8, totalSupply uint64) *TokenContract {
    return &TokenContract{
        SmartContract: &SmartContract{
            Code:    []byte{},
            Storage: make(map[string][]byte),
        },
        Name:        name,
        Symbol:      symbol,
        Decimals:    decimals,
        TotalSupply: totalSupply,
        Balances:    make(map[string]uint64),
    }
}

// Transfer (with overflow check)
func (tc *TokenContract) Transfer(from, to string, amount uint64) error {
    fromBalance := tc.Balances[from]
    
    // Balance check
    if fromBalance < amount {
        return fmt.Errorf("insufficient balance")
    }
    
    // Safe subtraction
    tc.Balances[from] -= amount
    
    // Safe addition (check overflow)
    toBalance := tc.Balances[to]
    if toBalance+amount < toBalance {
        return fmt.Errorf("balance overflow")
    }
    tc.Balances[to] += amount
    
    return nil
}

func (tc *TokenContract) BalanceOf(address string) uint64 {
    return tc.Balances[address]
}

func (tc *TokenContract) GetTotalSupply() uint64 {
    return tc.TotalSupply
}
```

#### 5.4.2 Multi-Signature Contract

```go
type MultiSigContract struct {
    *SmartContract
    Required int        // Required signatures
    PubKeys  []string   // Public key list
}

func NewMultiSigContract(required int, pubKeys []string) *MultiSigContract {
    // Validate parameters
    if required < 1 || required > len(pubKeys) {
        required = len(pubKeys)
    }
    
    return &MultiSigContract{
        SmartContract: &SmartContract{
            Code:    []byte{},
            Storage: make(map[string][]byte),
        },
        Required: required,
        PubKeys:  pubKeys,
    }
}

// Validate signatures
func (ms *MultiSigContract) ValidateSignatures(signatures [][]byte) bool {
    validCount := 0
    
    for _, sig := range signatures {
        if len(sig) > 0 {
            validCount++
        }
    }
    
    return validCount >= ms.Required
}

func (ms *MultiSigContract) GetRequired() int {
    return ms.Required
}

func (ms *MultiSigContract) GetPubKeys() []string {
    return ms.PubKeys
}
```

---

## 6. AI Audit

### 6.1 Smart Contract Analysis

#### 6.1.1 Security Rules

```go
type SecurityRule struct {
    Name           string  // Rule name
    Severity       string  // Severity (critical/high/medium/low)
    Pattern        string  // Regex pattern
    Description    string  // Description
    Recommendation string  // Fix recommendation
}

var contractSecurityRules = []SecurityRule{
    {
        Name:           "Reentrancy",
        Severity:       "high",
        Pattern:        "(?i)(call|callvalue|delegatecall).*balance.*transfer",
        Description:    "Potential reentrancy vulnerability detected",
        Recommendation: "Use checks-effects-interactions pattern or reentrancy guard",
    },
    {
        Name:           "Integer Overflow",
        Severity:       "high",
        Pattern:        "(?i)(add|sub|mul).*(overflow|underflow)",
        Description:    "Potential integer overflow/underflow",
        Recommendation: "Use SafeMath or Solidity 0.8+ checked math",
    },
    {
        Name:           "Selfdestruct",
        Severity:       "high",
        Pattern:        "(?i)selfdestruct|suicide",
        Description:    "Contract can be destroyed",
        Recommendation: "Ensure destruction is properly authorized",
    },
    {
        Name:           "Delegatecall",
        Severity:       "critical",
        Pattern:        "(?i)delegatecall",
        Description:    "Using delegatecall with untrusted code is dangerous",
        Recommendation: "Ensure delegatecall target is trusted and immutable",
    },
    {
        Name:           "tx.origin Usage",
        Severity:       "medium",
        Pattern:        "(?i)tx\\.origin",
        Description:    "Using tx.origin for authorization is vulnerable to phishing",
        Recommendation: "Use msg.sender instead",
    },
}
```

#### 6.1.2 Analysis Results

```go
type AnalysisResult struct {
    IsSafe      bool            // Is safe
    Issues      []SecurityIssue // Issues found
    Summary     string          // Summary
    SafeOpcodes []string        // Safe opcodes
    RiskScore   float64         // Risk score (0-100)
}

type SecurityIssue struct {
    Type           string  // Issue type
    Severity       string  // Severity
    Location       string  // Location
    Description    string  // Description
    Recommendation string  // Recommendation
}
```

#### 6.1.3 Bytecode Analysis

```go
type ContractAnalyzer struct {
    rules []SecurityRule
}

func NewContractAnalyzer() *ContractAnalyzer {
    rules := make([]SecurityRule, len(contractSecurityRules))
    copy(rules, contractSecurityRules)
    return &ContractAnalyzer{rules: rules}
}

// Analyze bytecode
func (ca *ContractAnalyzer) Analyze(bytecode string) *AnalysisResult {
    result := &AnalysisResult{
        IsSafe:      true,
        Issues:      []SecurityIssue{},
        SafeOpcodes: []string{},
    }
    
    bytecodeLower := strings.ToLower(bytecode)
    
    // Check dangerous opcodes
    vulnerableOpcodes := map[string]string{
        "callcode":     "delegatecall-like behavior",
        "delegate":     "delegatecall execution",
        "selfdestruct": "contract destruction",
        "suicide":      "contract destruction",
    }
    
    for opcode, issue := range vulnerableOpcodes {
        if strings.Contains(bytecodeLower, opcode) {
            result.IsSafe = false
            result.Issues = append(result.Issues, SecurityIssue{
                Type:           opcode,
                Severity:       "high",
                Location:       "bytecode",
                Description:    "Vulnerable opcode detected: " + issue,
                Recommendation: "Review and audit the contract",
            })
            result.RiskScore += 40
        }
    }
    
    // Check safe opcodes
    safeOpcodes := map[string]string{
        "push":  "Push operation",
        "pop":   "Pop operation",
        "add":   "Addition",
        "sub":   "Subtraction",
        "mul":   "Multiplication",
        "div":   "Division",
        "stop":  "Stop execution",
        "swap":  "Stack swap",
        "dup":   "Stack duplication",
        "jump":  "Conditional jump",
        "jumpi": "Conditional jump",
        "store": "Storage write",
        "load":  "Storage read",
    }
    
    for opcode := range safeOpcodes {
        if strings.Contains(bytecodeLower, opcode) {
            result.SafeOpcodes = append(result.SafeOpcodes, opcode)
        }
    }
    
    // Generate summary
    if len(result.Issues) == 0 {
        result.Summary = "No obvious vulnerabilities detected in bytecode"
    } else {
        result.Summary = "Contract analysis complete"
    }
    
    // Risk score threshold
    if result.RiskScore > 70 {
        result.IsSafe = false
    } else if result.RiskScore > 30 {
        result.IsSafe = false
    }
    
    return result
}
```

#### 6.1.4 Source Code Analysis

```go
// Analyze source code
func (ca *ContractAnalyzer) AnalyzeSource(sourceCode string) *AnalysisResult {
    result := &AnalysisResult{
        IsSafe:      true,
        Issues:      []SecurityIssue{},
        SafeOpcodes: []string{},
    }
    
    if sourceCode == "" {
        result.Summary = "No source code provided"
        return result
    }
    
    // Apply security rules
    for _, rule := range ca.rules {
        re := regexp.MustCompile(rule.Pattern)
        if re.MatchString(sourceCode) {
            result.Issues = append(result.Issues, SecurityIssue{
                Type:           rule.Name,
                Severity:       rule.Severity,
                Location:       "source",
                Description:    rule.Description,
                Recommendation: rule.Recommendation,
            })
            
            // Increase risk score based on severity
            switch rule.Severity {
            case "critical":
                result.RiskScore += 50
            case "high":
                result.RiskScore += 30
            case "medium":
                result.RiskScore += 15
            case "low":
                result.RiskScore += 5
            }
        }
    }
    
    // Generate results
    if len(result.Issues) == 0 {
        result.Summary = "No security issues detected"
        result.IsSafe = true
    } else {
        result.IsSafe = result.RiskScore < 50
        result.Summary = "Found issues requiring review"
    }
    
    return result
}
```

#### 6.1.5 Comprehensive Security Score

```go
// Get security score
func (ca *ContractAnalyzer) GetSecurityScore(bytecode, sourceCode string) float64 {
    result := ca.Analyze(bytecode)
    
    if sourceCode != "" {
        sourceResult := ca.AnalyzeSource(sourceCode)
        result.Issues = append(result.Issues, sourceResult.Issues...)
        result.RiskScore = (result.RiskScore + sourceResult.RiskScore) / 2
    }
    
    return result.RiskScore
}

// Check common vulnerabilities
func (ca *ContractAnalyzer) CheckCommonVulnerabilities(bytecode, sourceCode string) map[string]bool {
    checks := map[string]bool{
        "reentrancy_safe":       true,
        "overflow_safe":         true,
        "access_control_secure": true,
        "tx_origin_safe":        true,
        "unchecked_calls_safe":  true,
    }
    
    result := ca.Analyze(bytecode)
    if sourceCode != "" {
        sourceResult := ca.AnalyzeSource(sourceCode)
        result.Issues = append(result.Issues, sourceResult.Issues...)
    }
    
    for _, issue := range result.Issues {
        switch issue.Type {
        case "Reentrancy":
            checks["reentrancy_safe"] = false
        case "Integer Overflow":
            checks["overflow_safe"] = false
        case "Access Control":
            checks["access_control_secure"] = false
        case "tx.origin Usage":
            checks["tx_origin_safe"] = false
        case "Unchecked Call Return":
            checks["unchecked_calls_safe"] = false
        }
    }
    
    return checks
}
```

---

## 7. Governance and Upgrades

### 7.1 Protocol Version

```go
type ProtocolVersion struct {
    Major uint64 `json:"major"`  // Major version (breaking changes)
    Minor uint64 `json:"minor"`  // Minor version (backward-compatible features)
    Patch uint64 `json:"patch"`  // Patch version (backward-compatible fixes)
}

type UpgradeProposal struct {
    ID          string          `json:"id"`           // Proposal ID
    Title       string          `json:"title"`        // Proposal title
    Description string          `json:"description"`  // Proposal description
    Version     ProtocolVersion `json:"version"`      // Target version
    Proposer    string          `json:"proposer"`     // Proposer
    Timestamp   int64           `json:"timestamp"`    // Proposal timestamp
    Status      string          `json:"status"`       // Status (pending/voting/passed/rejected)
}

type ProtocolUpgrade struct {
    Version        ProtocolVersion `json:"version"`
    ActivationHeight uint64        `json:"activationHeight"`  // Activation height
    Description    string          `json:"description"`
    Features       []string        `json:"features"`
}
```

### 7.2 Upgrade Mechanism

```go
type UpgradeJSON struct {
    Version            string `json:"version"`
    ActivationHeight   uint64 `json:"activationHeight"`
    Description        string `json:"description"`
    ConsensusParams    struct {
        DifficultyEnable           bool   `json:"difficultyEnable"`
        TargetBlockTimeMs          int64  `json:"targetBlockTimeMs"`
        // ...
    } `json:"consensusParams"`
}

// Validate upgrade
func ValidateUpgrade(upgrade UpgradeJSON) error {
    if upgrade.Version == "" {
        return errors.New("version is required")
    }
    if upgrade.ActivationHeight == 0 {
        return errors.New("activationHeight is required")
    }
    if upgrade.Description == "" {
        return errors.New("description is required")
    }
    return nil
}
```

---

## 8. Monitoring Metrics

### 8.1 Prometheus Metrics

NogoChain uses `prometheus/client_golang` to expose key metrics:

**Block Metrics**:
- `nogo_block_height` - Current block height
- `nogo_block_time` - Block generation time interval
- `nogo_block_difficulty` - Current difficulty
- `nogo_block_tx_count` - Number of transactions per block

**Transaction Metrics**:
- `nogo_mempool_size` - Mempool size
- `nogo_mempool_fee_avg` - Average transaction fee
- `nogo_transaction_count` - Total transaction count

**Network Metrics**:
- `nogo_peer_count` - Number of connected peers
- `nogo_peer_score_avg` - Average peer score
- `nogo_network_bytes_sent` - Bytes sent
- `nogo_network_bytes_received` - Bytes received

**System Metrics**:
- `nogo_chain_work` - Cumulative work
- `nogo_state_size` - State size

### 8.2 Logging System

**Structured Logging (JSON Format)**:

```go
type LogEntry struct {
    Timestamp string `json:"timestamp"`
    Level     string `json:"level"`      // debug/info/warn/error
    Message   string `json:"message"`
    TraceID   string `json:"trace_id,omitempty"`
    
    // Context information
    BlockHeight uint64 `json:"block_height,omitempty"`
    TxID        string `json:"tx_id,omitempty"`
    Peer        string `json:"peer,omitempty"`
    Duration    int64  `json:"duration_ms,omitempty"`
}
```

**Log Levels**:
- `DEBUG`: Debug information (development environment)
- `INFO`: General information (production environment default)
- `WARN`: Warning information (needs attention but doesn't affect operation)
- `ERROR`: Error information (requires immediate handling)

**Sensitive Information Protection**:
- Prohibit printing private keys, passwords, tokens, and other sensitive information
- Addresses can be selectively desensitized when displayed
- Do not log complete network request payloads

---

## Appendix

### A. Environment Variable Configuration

| Variable | Description | Default Value |
|----------|-------------|---------------|
| `CHAIN_ID` | Chain ID | 1 |
| `MINER_ADDRESS` | Miner address | - |
| `PEERS` | Peer list | - |
| `DIFFICULTY_ENABLE` | Enable difficulty adjustment | false |
| `DIFFICULTY_TARGET_MS` | Target block time (milliseconds) | 15000 |
| `DIFFICULTY_WINDOW` | Difficulty adjustment window | 20 |
| `MAX_BLOCK_SIZE` | Maximum block size | 1000000 |
| `MERKLE_ENABLE` | Enable Merkle tree | false |
| `BINARY_ENCODING_ENABLE` | Enable binary encoding | false |

### B. File Structure

```
data/
├── blocks.dat       # Block data
├── state.dat        # State data
├── genesis_hash.txt # Genesis block hash
└── rules_hash.txt   # Consensus rules hash

keystore/
└── UTC--<address>   # Encrypted wallet file

genesis.json         # Genesis block configuration
```

### C. API Endpoints

**HTTP API**:
- `GET /chain/info` - Chain info
- `GET /blocks/height/:height` - Query block by height
- `GET /blocks/hash/:hash` - Query block by hash
- `GET /tx/:txid` - Query transaction
- `POST /tx` - Submit transaction
- `GET /balance/:address` - Query balance
- `GET /address/:address/txs` - Address transaction history

**WebSocket**:
- `ws://<host>/ws` - Real-time event subscription
  - Subscription types: `new_block`, `new_tx`, `address:<address>`

---

**Documentation Version**: 1.0  
**Last Updated**: 2026-04-01  
**Maintainer**: NogoChain Development Team
