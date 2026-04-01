# NogoChain 关键技术模块文档

**版本**: 1.0  
**生成日期**: 2026-04-01  
**文档语言**: 简体中文

---

## 目录

1. [钱包实现](#1-钱包实现)
2. [交易机制](#2-交易机制)
3. [区块结构](#3-区块结构)
4. [网络协议](#4-网络协议)
5. [智能合约](#5-智能合约)
6. [AI 审计](#6-ai-审计)
7. [治理与升级](#7-治理与升级)
8. [监控指标](#8-监控指标)

---

## 1. 钱包实现

### 1.1 账户模型

#### 1.1.1 Ed25519 密钥对

NogoChain 使用 Ed25519 数字签名算法，提供高安全性和高性能。

**核心结构**:

```go
type Wallet struct {
    PrivateKey ed25519.PrivateKey  // Ed25519 私钥（64 字节）
    PublicKey  ed25519.PublicKey   // Ed25519 公钥（32 字节）
    Address    string              // 地址（SHA256 公钥的十六进制编码）
}
```

**地址生成算法**:

```go
func GenerateAddress(pubKey ed25519.PublicKey) string {
    sum := sha256.Sum256(pubKey)
    return hex.EncodeToString(sum[:])  // 64 字符十六进制字符串
}
```

**代码示例 - 创建新钱包**:

```go
// 生成随机密钥对
pub, priv, err := ed25519.GenerateKey(rand.Reader)
if err != nil {
    return nil, err
}

// 创建钱包
wallet := &Wallet{
    PrivateKey: priv,
    PublicKey:  pub,
    Address:    GenerateAddress(pub),
}
```

#### 1.1.2 地址格式

- **格式**: 64 字符十六进制字符串
- **示例**: `NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf`
- **验证**: 支持两种格式验证
  - 原始十六进制：直接验证十六进制编码
  - NOGO00 前缀格式：验证前缀后验证剩余部分

### 1.2 HD 钱包（分层确定性钱包）

#### 1.2.1 BIP39 助记词

**核心参数**:

```go
const (
    MnemonicWordCount      = 12  // 助记词数量
    MnemonicEntropyBits    = 128 // 熵大小（128 位）
    MnemonicChecksumBits   = 4   // 校验和大小（128/32）
)
```

**助记词生成流程**:

```
1. 生成 128 位随机熵 (16 字节)
2. 计算 SHA256 校验和（取前 4 位）
3. 熵 + 校验和 = 132 位
4. 每 11 位映射到一个单词（共 12 个单词）
5. 使用 BIP39 英文词表（2048 词）
```

**代码示例 - 生成助记词**:

```go
func GenerateMnemonic() (string, error) {
    // 生成随机熵
    entropy := make([]byte, 16)
    _, err := rand.Read(entropy)
    if err != nil {
        return "", fmt.Errorf("failed to generate entropy: %w", err)
    }
    
    return EntropyToMnemonic(entropy)
}

// 熵转助记词
func EntropyToMnemonic(entropy []byte) (string, error) {
    // 计算校验和
    hash := sha256.Sum256(entropy)
    
    // 创建位数组（熵 + 校验和）
    totalBits := 128 + 4
    bitArray := make([]bool, totalBits)
    
    // 复制熵位
    for i := 0; i < 128; i++ {
        byteIndex := i / 8
        bitIndex := 7 - (i % 8)
        bitArray[i] = (entropy[byteIndex] & (1 << bitIndex)) != 0
    }
    
    // 复制校验和位
    for i := 0; i < 4; i++ {
        bitArray[128+i] = (hash[0] & (1 << (7 - i))) != 0
    }
    
    // 每 11 位转换为一个单词索引
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

**助记词验证**:

```go
func ValidateMnemonic(mnemonic string) bool {
    _, err := MnemonicToEntropy(mnemonic)
    return err == nil
}

// 助记词转熵（验证校验和）
func MnemonicToEntropy(mnemonic string) ([]byte, error) {
    words := strings.Fields(mnemonic)
    
    if len(words) != 12 {
        return nil, fmt.Errorf("invalid word count: %d", len(words))
    }
    
    // 验证每个单词并转换为索引
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
    
    // 转换为位数组并验证校验和
    // ...
}
```

#### 1.2.2 BIP32 派生

**HD 钱包结构**:

```go
type HDWallet struct {
    PrivateKey ed25519.PrivateKey  // 当前层级私钥
    PublicKey  ed25519.PublicKey   // 当前层级公钥
    ChainCode  []byte              // 链码（32 字节）
    Depth      uint8               // 深度（从 0 开始）
    Index      uint32              // 索引号
    Parent     []byte              // 父密钥指纹
}
```

**派生路径解析**:

```go
// 支持标准路径格式：m/44'/0'/0'/0/0
func parsePath(path string) []uint32 {
    var result []uint32
    
    // 移除 "m/" 前缀
    if len(path) >= 2 && path[:2] == "m/" {
        path = path[2:]
    }
    
    // 解析每个层级
    segments := splitPath(path)
    for _, seg := range segments {
        isHardened := len(seg) > 0 && seg[len(seg)-1] == '\''
        var index uint32
        
        if isHardened {
            // 硬化派生：index = 正常索引 + 0x80000000
            numStr := seg[:len(seg)-1]
            fmt.Sscanf(numStr, "%d", &index)
            index += BIP32HardenedBase  // 0x80000000
        } else {
            // 正常派生
            fmt.Sscanf(seg, "%d", &index)
        }
        result = append(result, index)
    }
    
    return result
}
```

**密钥派生算法**:

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
        
        // 准备派生数据
        data := make([]byte, 37)
        if isHardened {
            // 硬化派生：0x00 + 私钥
            data[0] = 0x00
            copy(data[1:33], w.PrivateKey[32:])
        } else {
            // 正常派生：公钥
            copy(data, w.PublicKey)
        }
        binary.BigEndian.PutUint32(data[33:37], index)
        
        // HMAC-SHA512(ChainCode, Data)
        hmac := sha512.New()
        hmac.Write(w.ChainCode)
        hmac.Write(data)
        I := hmac.Sum(nil)
        
        // 分割结果
        childKey := make([]byte, 32)
        childChainCode := make([]byte, 32)
        copy(childKey, I[:32])
        copy(childChainCode, I[32:64])
        
        // 子私钥 = 父私钥 + IL (mod n)
        for j := range childKey {
            childKey[j] ^= w.PrivateKey[j]
        }
        
        // 创建子钱包
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

**从助记词创建 HD 钱包**:

```go
func NewHDWallet(seed []byte) (*HDWallet, error) {
    // BIP39 种子转 HD 钱包种子
    // 使用 HMAC-SHA512("ed25519 seed", seed)
    hmac := sha512.New()
    hmac.Write([]byte("ed25519 seed"))
    hmac.Write(seed)
    I := hmac.Sum(nil)
    
    // IL 作为主私钥，IR 作为主链码
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

#### 1.2.3 助记词转种子

```go
func MnemonicToSeed(mnemonic, passphrase string) ([]byte, error) {
    // 标准化助记词（NFKD 规范化）
    normalizedMnemonic := strings.ToLower(strings.TrimSpace(mnemonic))
    
    // 创建盐值："mnemonic" + passphrase
    salt := "mnemonic" + passphrase
    
    // PBKDF2 with HMAC-SHA512 (2048 次迭代)
    hmac := sha512.New()
    hmac.Write([]byte(salt))
    hmac.Write([]byte(normalizedMnemonic))
    seed := hmac.Sum(nil)
    
    // 迭代 2048 次
    for i := 0; i < 2047; i++ {
        hmac.Reset()
        hmac.Write(seed)
        seed = hmac.Sum(nil)
    }
    
    return seed, nil  // 返回 64 字节种子
}
```

### 1.3 密钥管理

#### 1.3.1 加密存储（Keystore）

**Keystore 文件结构**:

```go
type KeystoreFile struct {
    Version   int               `json:"version"`   // 版本号（当前为 1）
    Address   string            `json:"address"`   // 钱包地址
    KDF       KeystoreKDF       `json:"kdf"`       // 密钥派生函数参数
    Cipher    KeystoreCipher    `json:"cipher"`    // 加密参数
    EncryptedMnemonic *EncryptedMnemonic `json:"encryptedMnemonic,omitempty"` // 可选：加密的助记词
}

type KeystoreKDF struct {
    Name       string `json:"name"`       // KDF 名称（"pbkdf2-sha256"）
    Iterations int    `json:"iterations"` // 迭代次数（默认 600,000）
    SaltB64    string `json:"saltB64"`    // Base64 编码的盐值（16 字节）
}

type KeystoreCipher struct {
    Name          string `json:"name"`          // 加密算法（"aes-256-gcm"）
    NonceB64      string `json:"nonceB64"`      // Base64 编码的 nonce（12 字节）
    CiphertextB64 string `json:"ciphertextB64"` // Base64 编码的密文
}
```

**加密流程**:

```go
func WriteKeystore(path string, w *Wallet, password string, params KeystoreKDF) error {
    // 1. 生成随机盐值
    salt := make([]byte, 16)
    _, err := rand.Read(salt)
    
    // 2. PBKDF2-HMAC-SHA256 派生密钥
    key := pbkdf2HMACSHA256([]byte(password), salt, params.Iterations, 32)
    
    // 3. AES-256-GCM 加密
    block, err := aes.NewCipher(key)
    gcm, err := cipher.NewGCM(block)
    nonce := make([]byte, gcm.NonceSize())
    _, err = rand.Read(nonce)
    
    // 4. 绑定地址防止替换攻击
    addrBytes, _ := hex.DecodeString(w.Address)
    ciphertext := gcm.Seal(nil, nonce, []byte(w.PrivateKey), addrBytes)
    
    // 5. 写入 JSON 文件
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
    
    // 原子写入文件
    tmp := path + ".tmp"
    os.WriteFile(tmp, json.MarshalIndent(ks, "", "  "), 0o600)
    os.Rename(tmp, path)
}
```

**解密流程**:

```go
func WalletFromKeystore(path string, password string) (*Wallet, error) {
    // 1. 读取 Keystore 文件
    ks, err := ReadKeystore(path)
    
    // 2. 验证版本和参数
    if ks.Version != 1 {
        return nil, fmt.Errorf("unsupported version: %d", ks.Version)
    }
    
    // 3. 解码参数
    salt, _ := base64.StdEncoding.DecodeString(ks.KDF.SaltB64)
    nonce, _ := base64.StdEncoding.DecodeString(ks.Cipher.NonceB64)
    ciphertext, _ := base64.StdEncoding.DecodeString(ks.Cipher.CiphertextB64)
    
    // 4. 派生解密密钥
    key := pbkdf2HMACSHA256([]byte(password), salt, ks.KDF.Iterations, 32)
    
    // 5. AES-GCM 解密
    block, _ := aes.NewCipher(key)
    gcm, _ := cipher.NewGCM(block)
    addrBytes, _ := hex.DecodeString(ks.Address)
    plain, err := gcm.Open(nil, nonce, ciphertext, addrBytes)
    
    if err != nil {
        return nil, ErrKeystoreWrongPassword
    }
    
    // 6. 从私钥字节创建钱包
    return WalletFromPrivateKeyBytes(plain)
}
```

**PBKDF2-HMAC-SHA256 实现**:

```go
func pbkdf2HMACSHA256(password, salt []byte, iter, keyLen int) []byte {
    hLen := 32  // SHA256 输出长度
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

#### 1.3.2 助记词备份

**加密存储助记词**:

```go
func WriteKeystoreWithMnemonic(path string, w *Wallet, mnemonic, password string, params KeystoreKDF) error {
    // 1. 写入基础 Keystore
    WriteKeystore(path, w, password, params)
    
    // 2. 单独加密助记词（使用不同的盐值）
    salt := make([]byte, 16)
    rand.Read(salt)
    
    mnemonicKey := pbkdf2HMACSHA256([]byte(password), salt, params.Iterations, 32)
    
    block, _ := aes.NewCipher(mnemonicKey)
    gcm, _ := cipher.NewGCM(block)
    nonce := make([]byte, gcm.NonceSize())
    rand.Read(nonce)
    
    ciphertext := gcm.Seal(nil, nonce, []byte(mnemonic), nil)
    
    // 3. 存储到 Keystore
    ks, _ := ReadKeystore(path)
    ks.EncryptedMnemonic = &EncryptedMnemonic{
        SaltB64:      base64.StdEncoding.EncodeToString(salt),
        NonceB64:     base64.StdEncoding.EncodeToString(nonce),
        CiphertextB64: base64.StdEncoding.EncodeToString(ciphertext),
    }
    
    // 4. 原子写入
    tmp := path + ".tmp"
    os.WriteFile(tmp, json.MarshalIndent(ks, "", "  "), 0o600)
    os.Rename(tmp, path)
}
```

**导出助记词**:

```go
func ExportMnemonicFromKeystore(path string, password string) (string, error) {
    ks, err := ReadKeystore(path)
    if err != nil {
        return "", err
    }
    
    if ks.EncryptedMnemonic == nil {
        return "", errors.New("no mnemonic stored")
    }
    
    // 解密助记词
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

### 1.4 钱包操作

#### 1.4.1 创建钱包

```go
// 方法 1：随机创建
func NewWallet() (*Wallet, error) {
    pub, priv, err := ed25519.GenerateKey(rand.Reader)
    return &Wallet{
        PrivateKey: priv,
        PublicKey:  pub,
        Address:    GenerateAddress(pub),
    }, nil
}

// 方法 2：从助记词创建
func CreateWalletFromMnemonic(mnemonic, passphrase string) (*Wallet, string, error) {
    // 如果没有提供助记词，生成新的
    if mnemonic == "" {
        mnemonic, err = GenerateMnemonic()
    }
    
    // 验证助记词
    if !ValidateMnemonic(mnemonic) {
        return nil, "", errors.New("invalid mnemonic")
    }
    
    // 助记词转种子
    seed, err := MnemonicToSeed(mnemonic, passphrase)
    
    // 创建 HD 钱包并派生
    hdWallet, _ := NewHDWallet(seed)
    derived, _ := hdWallet.Derive("m/0")
    
    return &Wallet{
        PrivateKey: derived.PrivateKey,
        PublicKey:  derived.PublicKey,
        Address:    GenerateAddress(derived.PublicKey),
    }, mnemonic, nil
}
```

#### 1.4.2 导入钱包

```go
// 从私钥 Base64 导入
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

// 从 Keystore 导入
func WalletFromKeystore(path string, password string) (*Wallet, error) {
    // 见 1.3.1 节
}
```

#### 1.4.3 导出钱包

```go
// 导出私钥 Base64
func (w *Wallet) PrivateKeyBase64() string {
    return base64.StdEncoding.EncodeToString(w.PrivateKey)
}

// 导出公钥 Base64
func (w *Wallet) PublicKeyBase64() string {
    return base64.StdEncoding.EncodeToString(w.PublicKey)
}

// 导出助记词（仅当从助记词创建时）
func WalletToMnemonic(w *Wallet) (string, error) {
    return "", errors.New("cannot export mnemonic: wallet was not created from mnemonic")
}
```

#### 1.4.4 交易签名

```go
func (w *Wallet) SignTransaction(tx *Transaction) error {
    // 1. 计算交易哈希
    hash, err := tx.SigningHash()
    if err != nil {
        return err
    }
    
    // 2. Ed25519 签名
    signature := ed25519.Sign(w.PrivateKey, hash)
    
    // 3. 设置签名和公钥
    tx.Signature = signature
    tx.FromPubKey = w.PublicKey
    
    return nil
}
```

---

## 2. 交易机制

### 2.1 交易结构

#### 2.1.1 交易类型定义

```go
type TransactionType string

const (
    TxCoinbase TransactionType = "coinbase"  // 区块奖励交易
    TxTransfer TransactionType = "transfer"  // 转账交易
)

type Transaction struct {
    Type       TransactionType `json:"type"`        // 交易类型
    ChainID    uint64          `json:"chainId"`     // 链 ID（防重放攻击）
    
    FromPubKey []byte          `json:"fromPubKey,omitempty"`  // 发送方公钥（Base64）
    ToAddress  string          `json:"toAddress"`             // 接收方地址
    
    Amount     uint64          `json:"amount"`      // 转账金额
    Fee        uint64          `json:"fee"`         // 交易费用
    Nonce      uint64          `json:"nonce,omitempty"`       // 发送方交易序号
    
    Data       string          `json:"data,omitempty"`        // 附加数据（智能合约调用等）
    Signature  []byte          `json:"signature,omitempty"`   // 签名（Base64）
}
```

#### 2.1.2 账户状态

```go
type Account struct {
    Balance uint64 `json:"balance"`  // 账户余额
    Nonce   uint64 `json:"nonce"`    // 已使用交易序号
}
```

### 2.2 交易签名

#### 2.2.1 签名哈希计算（共识层）

**双重签名哈希机制**:

```go
// 共识层签名哈希（支持分叉升级）
func txSigningHashForConsensus(tx Transaction, p ConsensusParams, height uint64) ([]byte, error) {
    if p.BinaryEncodingActive(height) {
        // 二进制编码（新版本）
        return txSigningHashBinary(tx)
    }
    // JSON 编码（旧版本，向后兼容）
    return tx.signingHashLegacyJSON()
}
```

**JSON 编码哈希（Legacy）**:

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
    
    // 转账交易包含发送方地址
    if t.Type == TxTransfer {
        fromAddr, _ := t.FromAddress()
        v.FromAddr = fromAddr
    }
    
    // JSON 序列化后 SHA256
    b, err := json.Marshal(v)
    if err != nil {
        return nil, err
    }
    sum := sha256.Sum256(b)
    return sum[:], nil
}
```

**交易 ID 计算**:

```go
func TxIDHex(tx Transaction) (string, error) {
    h, err := tx.SigningHash()
    if err != nil {
        return "", err
    }
    return hex.EncodeToString(h), nil
}

// 共识层交易 ID
func TxIDHexForConsensus(tx Transaction, p ConsensusParams, height uint64) (string, error) {
    h, err := txSigningHashForConsensus(tx, p, height)
    if err != nil {
        return "", err
    }
    return hex.EncodeToString(h), nil
}
```

#### 2.2.2 Ed25519 签名

```go
// 签名交易
func (w *Wallet) SignTransaction(tx *Transaction) error {
    // 1. 计算签名哈希
    hash, err := tx.SigningHash()
    if err != nil {
        return err
    }
    
    // 2. Ed25519 签名（64 字节）
    signature := ed25519.Sign(w.PrivateKey, hash)
    
    // 3. 设置签名和公钥
    tx.Signature = signature
    tx.FromPubKey = w.PublicKey
    
    return nil
}

// 验证签名
func (t Transaction) verifyWithSigningHash(h []byte) error {
    if t.Type != TxTransfer {
        return errors.New("signature verification only for transfer transactions")
    }
    
    // 验证公钥长度
    if len(t.FromPubKey) != ed25519.PublicKeySize {
        return fmt.Errorf("invalid fromPubKey length: %d", len(t.FromPubKey))
    }
    
    // 验证签名长度
    if len(t.Signature) != ed25519.SignatureSize {
        return fmt.Errorf("invalid signature length: %d", len(t.Signature))
    }
    
    // Ed25519 验证
    if !ed25519.Verify(t.FromPubKey, h, t.Signature) {
        return errors.New("invalid signature")
    }
    
    return nil
}
```

### 2.3 交易验证

#### 2.3.1 基础验证

```go
func (t Transaction) Verify() error {
    switch t.Type {
    case TxCoinbase:
        // 验证 Coinbase 交易
        if t.ChainID == 0 {
            return errors.New("chainId must be set")
        }
        if t.Amount == 0 {
            return errors.New("coinbase amount must be > 0")
        }
        if err := validateAddress(t.ToAddress); err != nil {
            return fmt.Errorf("invalid toAddress: %w", err)
        }
        // Coinbase 不能有签名/公钥/Nonce/费用
        if t.FromPubKey != nil || t.Signature != nil || t.Nonce != 0 || t.Fee != 0 {
            return errors.New("coinbase must not include fromPubKey/signature/nonce/fee")
        }
        return nil
        
    case TxTransfer:
        // 验证转账交易
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
        
        // 验证签名
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

#### 2.3.2 共识验证

```go
func (t Transaction) VerifyForConsensus(p ConsensusParams, height uint64) error {
    switch t.Type {
    case TxCoinbase:
        return t.Verify()  // Coinbase 验证相同
        
    case TxTransfer:
        // 重新运行结构验证
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
        
        // 使用共识选择的签名哈希验证签名
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

#### 2.3.3 余额和 Nonce 验证

```go
func applyBlockToState(p ConsensusParams, state map[string]Account, b *Block) error {
    // ... 区块验证 ...
    
    for i, tx := range b.Transactions {
        switch tx.Type {
        case TxCoinbase:
            // Coinbase：增加接收方余额
            acct := state[tx.ToAddress]
            acct.Balance += tx.Amount
            state[tx.ToAddress] = acct
            
        case TxTransfer:
            fromAddr, err := tx.FromAddress()
            if err != nil {
                return err
            }
            
            from := state[fromAddr]
            
            // Nonce 必须连续递增
            if from.Nonce+1 != tx.Nonce {
                return fmt.Errorf("bad nonce for %s: expected %d got %d", 
                    fromAddr, from.Nonce+1, tx.Nonce)
            }
            
            // 余额检查（金额 + 费用）
            totalDebit := tx.Amount + tx.Fee
            if from.Balance < totalDebit {
                return fmt.Errorf("insufficient funds for %s", fromAddr)
            }
            
            // 扣款并更新 Nonce
            from.Balance -= totalDebit
            from.Nonce = tx.Nonce
            state[fromAddr] = from
            
            // 收款
            to := state[tx.ToAddress]
            to.Balance += tx.Amount
            state[tx.ToAddress] = to
        }
    }
    return nil
}
```

### 2.4 费用计算

#### 2.4.1 最小费用

```go
const (
    minFee = uint64(1)  // 最小交易费用
)

// 验证费用
if tx.Fee < minFee {
    return nil, fmt.Errorf("fee too low: minFee=%d", minFee)
}
```

#### 2.4.2 矿工费用分配

```go
// 矿工费用计算
func (bc *Blockchain) MineTransfers(transfers []Transaction) (*Block, error) {
    var fees uint64
    for _, tx := range transfers {
        fees += tx.Fee
    }
    
    // 获取货币政策
    policy := bc.consensus.MonetaryPolicy
    
    // 区块奖励（根据高度计算）
    reward := policy.BlockReward(height)
    
    // 矿工费用份额
    minerFees := policy.MinerFeeAmount(fees)
    
    // Coinbase 交易
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

### 2.5 交易池管理

#### 2.5.1 交易池结构

```go
type mempoolEntry struct {
    tx       Transaction   // 交易
    txIDHex  string        // 交易 ID
    received time.Time     // 接收时间
}

type Mempool struct {
    mu              sync.Mutex
    maxSize         int
    entries         map[string]mempoolEntry       // txid -> entry
    bySenderNonce   map[string]map[uint64]string  // fromAddr -> nonce -> txid
}
```

#### 2.5.2 添加交易

```go
func (m *Mempool) AddWithTxID(tx Transaction, txid string, p ConsensusParams, height uint64) (string, error) {
    // 计算交易 ID
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
    
    // 获取发送方地址
    fromAddr, err := tx.FromAddress()
    if err != nil {
        return "", err
    }
    
    m.mu.Lock()
    defer m.mu.Unlock()
    
    // 检查重复
    if _, ok := m.entries[txid]; ok {
        return txid, errors.New("duplicate transaction")
    }
    
    // 检查 Nonce 冲突
    if existingID, ok := m.bySenderNonce[fromAddr][tx.Nonce]; ok {
        return existingID, errors.New("nonce already in mempool")
    }
    
    // 容量检查（满时驱逐最低费用交易）
    if len(m.entries) >= m.maxSize {
        lowest := m.lowestFeeLocked()
        if lowest == "" {
            return "", errors.New("mempool full")
        }
        m.evictWithDependentsLocked(lowest)
    }
    
    // 添加交易
    m.entries[txid] = mempoolEntry{tx: tx, txIDHex: txid, received: time.Now()}
    m.indexLocked(fromAddr, tx.Nonce, txid)
    
    return txid, nil
}
```

#### 2.5.3 Replace-By-Fee (RBF)

```go
func (m *Mempool) ReplaceByFeeWithTxID(tx Transaction, txid string, p ConsensusParams, height uint64) (string, bool, []string, error) {
    // 计算交易 ID
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
    
    // 检查是否存在相同 Nonce 的交易
    existingID, ok := m.bySenderNonce[fromAddr][tx.Nonce]
    if !ok {
        return "", false, nil, errors.New("no existing nonce to replace")
    }
    
    existing := m.entries[existingID]
    
    // 新费用必须严格大于旧费用
    if tx.Fee <= existing.tx.Fee {
        return "", false, nil, errors.New("replacement fee must be higher")
    }
    
    // 驱逐旧交易及其依赖（更高 Nonce 的交易）
    evicted := m.evictBySenderNonceLocked(fromAddr, tx.Nonce)
    
    // 插入新交易
    m.entries[txid] = mempoolEntry{tx: tx, txIDHex: txid, received: time.Now()}
    m.indexLocked(fromAddr, tx.Nonce, txid)
    
    return txid, true, evicted, nil
}

// 驱逐指定 Nonce 及更高 Nonce 的交易
func (m *Mempool) evictBySenderNonceLocked(fromAddr string, nonce uint64) []string {
    var evicted []string
    
    // 驱逐目标 Nonce
    if txid, ok := m.bySenderNonce[fromAddr][nonce]; ok {
        evicted = append(evicted, txid)
        m.removeLocked(txid)
    }
    
    // 驱逐所有更高 Nonce 的交易
    for n, txid := range m.bySenderNonce[fromAddr] {
        if n > nonce {
            evicted = append(evicted, txid)
            m.removeLocked(txid)
        }
    }
    
    return evicted
}
```

#### 2.5.4 交易选择（出块）

```go
func (bc *Blockchain) SelectMempoolTxs(mp *Mempool, max int) ([]Transaction, []string, error) {
    if max <= 0 {
        max = 100
    }
    
    // 复制当前状态
    bc.mu.RLock()
    baseState := make(map[string]Account, len(bc.state))
    for k, v := range bc.state {
        baseState[k] = v
    }
    bc.mu.RUnlock()
    
    // 按费用降序获取交易
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
        
        // 验证交易
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
        
        // Nonce 必须连续
        if from.Nonce+1 != tx.Nonce {
            continue
        }
        
        // 余额检查
        totalDebit := tx.Amount + tx.Fee
        if from.Balance < totalDebit {
            continue
        }
        
        // 在模拟状态中应用交易（支持同一发送方的连续交易）
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

#### 2.5.5 交易池查询

```go
// 按费用降序返回所有交易
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

// 查询发送方的待处理交易（按 Nonce 排序）
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

// 查询指定 Nonce 的交易
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

## 3. 区块结构

### 3.1 区块头

#### 3.1.1 区块结构定义

```go
type Block struct {
    Version        uint32        `json:"version"`         // 区块版本
    Height         uint64        `json:"height"`          // 区块高度
    TimestampUnix  int64         `json:"timestampUnix"`   // Unix 时间戳
    PrevHash       []byte        `json:"prevHash"`        // 前块哈希（Base64）
    Nonce          uint64        `json:"nonce"`           // 工作量证明随机数
    DifficultyBits uint32        `json:"difficultyBits"`  // 难度目标
    MinerAddress   string        `json:"minerAddress"`    // 矿工地址
    Transactions   []Transaction `json:"transactions"`    // 交易列表
    Hash           []byte        `json:"hash"`            // 区块哈希（Base64）
}
```

#### 3.1.2 区块头哈希计算

**多版本区块头编码**:

```go
func (b *Block) HeaderBytesForConsensus(p ConsensusParams, nonce uint64) ([]byte, error) {
    // 检查是否启用二进制编码
    if p.BinaryEncodingActive(b.Height) {
        return blockHeaderPreimageBinaryV1(b, nonce, p)
    }
    
    switch b.Version {
    case 2:
        // V2 区块头（使用 Merkle 根）
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
        // V1 区块头（使用 TxRoot）
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

#### 3.1.3 难度调整

**ConsensusParams 结构**:

```go
type ConsensusParams struct {
    DifficultyEnable      bool          // 是否启用难度调整
    TargetBlockTime       time.Duration // 目标出块时间（默认 15 秒）
    DifficultyWindow      int           // 难度调整窗口（默认 20 块）
    DifficultyMaxStep     uint32        // 最大难度步进值
    MinDifficultyBits     uint32        // 最小难度
    MaxDifficultyBits     uint32        // 最大难度（256）
    GenesisDifficultyBits uint32        // 创世块难度
    MedianTimePastWindow  int           // 中位时间过去窗口
    MaxTimeDrift          int64         // 最大时间漂移（秒）
    MaxBlockSize          uint64        // 最大区块大小
    MerkleEnable          bool          // 是否启用 Merkle 树
    MerkleActivationHeight uint64       // Merkle 激活高度
    BinaryEncodingEnable  bool          // 是否启用二进制编码
    BinaryEncodingActivationHeight uint64 // 二进制编码激活高度
    MonetaryPolicy        MonetaryPolicy // 货币政策
}
```

**难度调整算法**:

```go
func nextDifficultyBitsFromPath(p ConsensusParams, path []*Block) uint32 {
    if len(path) == 0 {
        return p.GenesisDifficultyBits
    }
    
    parentIdx := len(path) - 1
    parent := path[parentIdx]
    
    // 未启用难度调整
    if !p.DifficultyEnable {
        if parent.DifficultyBits == 0 {
            return p.GenesisDifficultyBits
        }
        return clampDifficultyBits(p, parent.DifficultyBits)
    }
    
    // 自适应窗口大小
    windowSize := p.DifficultyWindow
    if parentIdx < windowSize {
        if parentIdx == 0 {
            // 第一块：使用创世难度
            return clampDifficultyBits(p, p.GenesisDifficultyBits)
        }
        windowSize = parentIdx
    }
    
    olderIdx := parentIdx - windowSize
    older := path[olderIdx]
    
    // 计算实际时间跨度
    actualSpanSec := parent.TimestampUnix - older.TimestampUnix
    if actualSpanSec <= 0 {
        actualSpanSec = 1
    }
    
    // 计算预期时间跨度
    targetSec := int64(p.TargetBlockTime / time.Second)
    if targetSec <= 0 {
        targetSec = 1
    }
    expectedSpanSec := int64(windowSize) * targetSec
    
    // 防止极端时间扭曲
    if actualSpanSec < expectedSpanSec/4 {
        actualSpanSec = expectedSpanSec / 4
    }
    if actualSpanSec > expectedSpanSec*4 {
        actualSpanSec = expectedSpanSec * 4
    }
    
    // 计算调整比例
    adjustmentRatio := float64(actualSpanSec) / float64(expectedSpanSec)
    
    next := float64(parent.DifficultyBits)
    
    // 百分比调整（敏感度 50%）
    const sensitivity = 0.5
    if adjustmentRatio < 1.0 {
        // 出块太快：增加难度
        increaseFactor := (1.0 - adjustmentRatio) * sensitivity
        next = next * (1.0 + increaseFactor)
    } else if adjustmentRatio > 1.0 {
        // 出块太慢：降低难度
        decreaseFactor := (adjustmentRatio - 1.0) * sensitivity
        next = next * (1.0 - decreaseFactor)
    }
    
    // 低难度时最小变化为 1
    if parent.DifficultyBits <= 10 {
        if adjustmentRatio < 1.0 && next <= float64(parent.DifficultyBits) {
            next = float64(parent.DifficultyBits) + 1
        } else if adjustmentRatio > 1.0 && next >= float64(parent.DifficultyBits) {
            if next > 1 {
                next = float64(parent.DifficultyBits) - 1
            }
        }
    }
    
    // 应用最大步进限制
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

// 难度范围限制
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

### 3.2 交易列表

#### 3.2.1 默克尔树（Merkle Tree）

**默克尔根计算**:

```go
// 计算默克尔根（Bitcoin 风格）
func MerkleRoot(leaves [][]byte) ([]byte, error) {
    if len(leaves) == 0 {
        return nil, errors.New("empty leaves")
    }
    
    // 叶子节点哈希：SHA256(0x00 || leaf)
    level := make([][]byte, 0, len(leaves))
    for _, l := range leaves {
        if len(l) != 32 {
            return nil, errors.New("leaf must be 32 bytes")
        }
        level = append(level, hashLeaf(l))
    }
    
    // 逐层计算
    for len(level) > 1 {
        next := make([][]byte, 0, (len(level)+1)/2)
        for i := 0; i < len(level); i += 2 {
            left := level[i]
            right := left  // 奇数时复制最后一个
            if i+1 < len(level) {
                right = level[i+1]
            }
            next = append(next, hashNode(left, right))
        }
        level = next
    }
    
    return append([]byte(nil), level[0]...), nil
}

// 叶子节点哈希：SHA256(0x00 || leaf)
func hashLeaf(leaf []byte) []byte {
    var b [1 + 32]byte
    b[0] = 0x00
    copy(b[1:], leaf)
    sum := sha256.Sum256(b[:])
    return sum[:]
}

// 内部节点哈希：SHA256(0x01 || left || right)
func hashNode(left, right []byte) []byte {
    var b [1 + 32 + 32]byte
    b[0] = 0x01
    copy(b[1:], left)
    copy(b[33:], right)
    sum := sha256.Sum256(b[:])
    return sum[:]
}
```

**区块默克尔根**:

```go
// V2 区块默克尔根
func (b *Block) MerkleRootV2ForConsensus(p ConsensusParams) ([]byte, error) {
    leaves := make([][]byte, 0, len(b.Transactions))
    
    // 交易签名哈希作为叶子
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

#### 3.2.2 默克尔证明

```go
// 生成默克尔证明
func MerkleProof(leaves [][]byte, index int) (branch [][]byte, siblingLeft []bool, root []byte, err error) {
    if len(leaves) == 0 {
        return nil, nil, nil, errors.New("empty leaves")
    }
    if index < 0 || index >= len(leaves) {
        return nil, nil, nil, errors.New("index out of range")
    }
    
    // 构建叶子层
    level := make([][]byte, 0, len(leaves))
    for _, l := range leaves {
        level = append(level, hashLeaf(l))
    }
    
    idx := index
    for len(level) > 1 {
        var sib []byte
        var sibIsLeft bool
        
        if idx%2 == 0 {
            // 兄弟节点在右边（或自身如果是复制的）
            sibIsLeft = false
            if idx+1 < len(level) {
                sib = level[idx+1]
            } else {
                sib = level[idx]
            }
        } else {
            // 兄弟节点在左边
            sibIsLeft = true
            sib = level[idx-1]
        }
        
        branch = append(branch, append([]byte(nil), sib...))
        siblingLeft = append(siblingLeft, sibIsLeft)
        
        // 计算下一层
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

// 验证默克尔证明
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

### 3.3 区块验证

#### 3.3.1 PoW 验证

**工作量证明验证**:

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
        // 创世块验证
        if i == 0 {
            if b.Height != 0 || len(b.PrevHash) != 0 {
                return errors.New("invalid genesis header")
            }
            if b.Version != blockVersionForHeight(consensus, 0) {
                return fmt.Errorf("bad block version at %d", b.Height)
            }
        } else {
            // 非创世块验证
            prev := blocks[i-1]
            
            // 高度连续
            if b.Height != prev.Height+1 {
                return fmt.Errorf("bad height at %d", b.Height)
            }
            
            // 前块哈希链接
            if string(b.PrevHash) != string(prev.Hash) {
                return fmt.Errorf("bad prev hash at %d", b.Height)
            }
            
            // 时间验证
            if err := validateBlockTime(consensus, blocks, i); err != nil {
                return err
            }
            
            // 难度验证
            if consensus.DifficultyEnable {
                expected := expectedDifficultyBitsForBlockIndex(consensus, blocks, i)
                if b.DifficultyBits != expected {
                    return fmt.Errorf("bad difficulty at %d: expected %d got %d", 
                        b.Height, expected, b.DifficultyBits)
                }
            }
            
            // 版本验证
            if b.Version != blockVersionForHeight(consensus, b.Height) {
                return fmt.Errorf("bad block version at %d", b.Height)
            }
        }
        
        // 难度范围验证
        if b.DifficultyBits == 0 || b.DifficultyBits > maxDifficultyBits {
            return fmt.Errorf("difficultyBits out of range at %d: %d", b.Height, b.DifficultyBits)
        }
        
        // PoW 验证
        ok, err := NewProofOfWork(consensus, b).Validate()
        if err != nil {
            return err
        }
        if !ok {
            return fmt.Errorf("invalid pow at height %d", b.Height)
        }
        
        // 交易验证
        for _, tx := range b.Transactions {
            if tx.ChainID == 0 {
                return fmt.Errorf("missing chainId at height %d", b.Height)
            }
            if err := tx.VerifyForConsensus(consensus, b.Height); err != nil {
                return fmt.Errorf("invalid tx at height %d: %w", b.Height, err)
            }
        }
    }
    
    // 状态可重现性验证
    return bc.recomputeState()
}
```

#### 3.3.2 时间验证

```go
func validateBlockTime(p ConsensusParams, blocks []*Block, idx int) error {
    if idx <= 0 || idx >= len(blocks) {
        return nil
    }
    
    current := blocks[idx]
    prev := blocks[idx-1]
    
    // 时间必须大于前块时间
    if current.TimestampUnix <= prev.TimestampUnix {
        return fmt.Errorf("block time must be greater than previous at %d", idx)
    }
    
    // 中位时间过去验证
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
    
    // 最大时间漂移验证
    maxFuture := time.Now().Unix() + p.MaxTimeDrift
    if current.TimestampUnix > maxFuture {
        return fmt.Errorf("block time too far in future at %d", idx)
    }
    
    return nil
}
```

#### 3.3.3 Coinbase 验证

```go
func applyBlockToState(p ConsensusParams, state map[string]Account, b *Block) error {
    // ...
    
    // 非创世块：验证 Coinbase 金额
    if b.Height > 0 {
        if err := validateAddress(b.MinerAddress); err != nil {
            return fmt.Errorf("invalid minerAddress: %w", err)
        }
        
        // 计算总费用
        var fees uint64
        for _, tx := range b.Transactions[1:] {
            if tx.Type != TxTransfer {
                continue
            }
            fees += tx.Fee
        }
        
        cb := b.Transactions[0]
        
        // Coinbase 收款地址必须匹配矿工地址
        if cb.ToAddress != b.MinerAddress {
            return errors.New("coinbase toAddress must match minerAddress")
        }
        
        // 验证 Coinbase 金额（区块奖励 + 矿工费用份额）
        policy := p.MonetaryPolicy
        expected := policy.BlockReward(b.Height) + policy.MinerFeeAmount(fees)
        
        if cb.Amount != expected {
            return fmt.Errorf("bad coinbase amount: expected %d got %d", expected, cb.Amount)
        }
    }
    
    // ...
}
```

### 3.4 区块索引

#### 3.4.1 交易索引

```go
type TxLocation struct {
    Height       uint64 `json:"height"`         // 区块高度
    BlockHashHex string `json:"blockHashHex"`   // 区块哈希（十六进制）
    Index        int    `json:"index"`          // 交易在区块中的索引
}

// 索引交易
func (bc *Blockchain) indexTxsForBlockLocked(b *Block) {
    if bc.txIndex == nil {
        bc.txIndex = map[string]TxLocation{}
    }
    
    hashHex := hex.EncodeToString(b.Hash)
    
    for i, tx := range b.Transactions {
        // 只索引转账交易（Coinbase 交易 ID 可能冲突）
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

// 查询交易
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

#### 3.4.2 地址索引

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

// 索引地址交易
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
        
        // 发送方索引
        bc.addressIndex[from] = append(bc.addressIndex[from], entry)
        
        // 接收方索引（如果不同）
        if tx.ToAddress != from {
            bc.addressIndex[tx.ToAddress] = append(bc.addressIndex[tx.ToAddress], entry)
        }
    }
}

// 查询地址交易历史（最新优先）
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
    
    // 从最新开始（数组末尾）
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

#### 3.4.3 高度索引

```go
type Blockchain struct {
    // ...
    blocks         []*Block              // 规范链（按高度排序）
    blocksByHash   map[string]*Block     // 哈希 -> 区块
    bestTipHash    string                // 最佳链尖哈希
    canonicalWork  *big.Int              // 规范链累计工作量
}

// 按高度查询区块
func (bc *Blockchain) BlockByHeight(height uint64) (*Block, bool) {
    bc.mu.RLock()
    defer bc.mu.RUnlock()
    
    if height >= uint64(len(bc.blocks)) {
        return nil, false
    }
    
    return bc.blocks[int(height)], true
}

// 按哈希查询区块
func (bc *Blockchain) BlockByHash(hashHex string) (*Block, bool) {
    bc.mu.RLock()
    defer bc.mu.RUnlock()
    
    block, ok := bc.blocksByHash[hashHex]
    if !ok {
        return nil, false
    }
    
    return block, true
}

// 获取最新区块
func (bc *Blockchain) LatestBlock() *Block {
    bc.mu.RLock()
    defer bc.mu.RUnlock()
    return bc.blocks[len(bc.blocks)-1]
}
```

---

## 4. 网络协议

### 4.1 P2P 通信

#### 4.1.1 消息格式

**P2P 信封结构**:

```go
type p2pEnvelope struct {
    Type    string          `json:"type"`     // 消息类型
    Payload json.RawMessage `json:"payload"`  // 消息负载
}
```

**消息类型**:

| 类型 | 描述 | 方向 |
|------|------|------|
| `hello` | 握手消息 | 双向 |
| `chain_info_req` | 链信息查询请求 | 客户端→服务器 |
| `chain_info` | 链信息响应 | 服务器→客户端 |
| `headers_from_req` | 区块头查询请求 | 客户端→服务器 |
| `headers` | 区块头响应 | 服务器→客户端 |
| `block_by_hash_req` | 区块查询请求 | 客户端→服务器 |
| `block` | 区块响应 | 服务器→客户端 |
| `tx_req` | 交易查询请求 | 客户端→服务器 |
| `tx_ack` | 交易确认 | 服务器→客户端 |
| `tx_broadcast` | 交易广播 | 客户端→服务器 |
| `tx_broadcast_ack` | 交易广播确认 | 服务器→客户端 |
| `block_broadcast` | 区块广播 | 客户端→服务器 |
| `block_broadcast_ack` | 区块广播确认 | 服务器→客户端 |
| `block_req` | 区块请求 | 客户端→服务器 |
| `getaddr` | 获取节点列表 | 客户端→服务器 |
| `addr` | 节点列表响应 | 服务器→客户端 |
| `addr_ack` | 节点列表确认 | 服务器→客户端 |
| `error` | 错误消息 | 双向 |

#### 4.1.2 编解码

**消息编码（TLV 格式）**:

```go
// 4 字节长度 + JSON 负载
func p2pWriteJSON(w io.Writer, v any) error {
    b, err := json.Marshal(v)
    if err != nil {
        return err
    }
    
    // 大端 4 字节长度
    var lenBuf [4]byte
    binary.BigEndian.PutUint32(lenBuf[:], uint32(len(b)))
    
    if _, err := w.Write(lenBuf[:]); err != nil {
        return err
    }
    
    _, err = w.Write(b)
    return err
}
```

**消息解码**:

```go
func p2pReadJSON(r io.Reader, maxBytes int) ([]byte, error) {
    // 读取 4 字节长度
    var lenBuf [4]byte
    if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
        return nil, err
    }
    
    n := int(binary.BigEndian.Uint32(lenBuf[:]))
    if n <= 0 {
        return nil, io.ErrUnexpectedEOF
    }
    
    // 大小限制
    if maxBytes > 0 && n > maxBytes {
        _, _ = io.CopyN(io.Discard, r, int64(n))
        return nil, ErrP2PTooLarge
    }
    
    // 读取负载
    b := make([]byte, n)
    if _, err := io.ReadFull(r, b); err != nil {
        return nil, err
    }
    
    return b, nil
}
```

#### 4.1.3 握手协议

**Hello 消息**:

```go
type p2pHello struct {
    Protocol  int    `json:"protocol"`   // 协议版本（当前为 1）
    ChainID   uint64 `json:"chainId"`    // 链 ID
    RulesHash string `json:"rulesHash"`  // 共识规则哈希
    NodeID    string `json:"nodeId"`     // 节点 ID
    TimeUnix  int64  `json:"timeUnix"`   // 时间戳
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

**握手流程**:

```go
func (s *P2PServer) handleConn(c net.Conn) error {
    defer c.Close()
    
    // 设置超时
    _ = c.SetDeadline(time.Now().Add(15 * time.Second))
    
    // 1. 接收 Hello
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
    
    // 2. 验证协议和链 ID
    if hello.Protocol != 1 || hello.ChainID != s.bc.ChainID {
        _ = p2pWriteJSON(c, p2pEnvelope{
            Type: "error", 
            Payload: mustJSON(map[string]any{"error": "wrong_chain_or_protocol"}),
        })
        return errors.New("wrong chain/protocol")
    }
    
    // 3. 验证共识规则哈希
    if hello.RulesHash != s.bc.RulesHashHex() {
        _ = p2pWriteJSON(c, p2pEnvelope{
            Type: "error",
            Payload: mustJSON(map[string]any{"error": "rules_hash_mismatch"}),
        })
        return errors.New("rules hash mismatch")
    }
    
    // 4. 回复 Hello
    _ = p2pWriteJSON(c, p2pEnvelope{
        Type: "hello",
        Payload: mustJSON(newP2PHello(s.bc.ChainID, s.bc.RulesHashHex(), s.nodeID)),
    })
    
    // 5. 处理请求
    raw, err = p2pReadJSON(c, s.maxMsgSize)
    // ...
}
```

### 4.2 节点发现

#### 4.2.1 节点评分

**PeerScore 结构**:

```go
type PeerScore struct {
    Peer           string      // 节点地址
    Score          float64     // 评分（0-100）
    SuccessCount   int         // 成功次数
    FailureCount   int         // 失败次数
    TotalLatencyMs int64       // 总延迟（毫秒）
    LastSeen       time.Time   // 最后活跃时间
    FirstSeen      time.Time   // 首次发现时间
}
```

**评分算法**:

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
    
    // 成功率
    successRate := float64(p.SuccessCount) / float64(total)
    
    // 平均延迟
    var avgLatency float64 = 1000
    if p.SuccessCount > 0 {
        avgLatency = float64(p.TotalLatencyMs) / float64(p.SuccessCount)
    }
    
    // 延迟因子
    latencyFactor := 1.0
    if avgLatency < 100 {
        latencyFactor = 1.5    // <100ms: 优秀
    } else if avgLatency < 500 {
        latencyFactor = 1.2    // <500ms: 良好
    } else if avgLatency > 2000 {
        latencyFactor = 0.5    // >2s: 较差
    } else if avgLatency > 5000 {
        latencyFactor = 0.2    // >5s: 很差
    }
    
    // 计算评分
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

**记录成功/失败**:

```go
func (ps *PeerScorer) RecordSuccess(peer string, latencyMs int64) {
    ps.mu.Lock()
    defer ps.mu.Unlock()
    
    now := time.Now()
    if p, ok := ps.peers[peer]; ok {
        // 更新现有节点
        p.SuccessCount++
        p.TotalLatencyMs += latencyMs
        p.LastSeen = now
        p.Score = ps.calculateScore(p)
    } else {
        // 添加新节点
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

**获取优质节点**:

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
    
    // 按评分降序排序
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

#### 4.2.2 节点管理

**PeerManager 结构**:

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

// 添加节点
func (pm *PeerManager) AddPeer(addr string) {
    if addr == "" {
        return
    }
    
    // 去重
    for _, p := range pm.peers {
        if p == addr {
            return
        }
    }
    
    pm.peers = append(pm.peers, addr)
}

// 解析节点列表（环境变量格式）
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

### 4.3 数据同步

#### 4.3.1 区块同步

**获取链信息**:

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

**获取区块头**:

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

**获取区块**:

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

**确保祖先区块**:

```go
func (pm *PeerManager) EnsureAncestors(ctx context.Context, bc *Blockchain, missingHashHex string) error {
    need := missingHashHex
    visited := map[string]struct{}{}
    
    for depth := 0; depth < pm.maxAncestorDepth; depth++ {
        // 本地已有
        if _, ok := bc.BlockByHash(need); ok {
            return nil
        }
        
        // 防止循环
        if _, ok := visited[need]; ok {
            return errors.New("ancestor fetch cycle")
        }
        visited[need] = struct{}{}
        
        // 获取区块
        b, _, err := pm.FetchAnyBlockByHash(ctx, need)
        if err != nil {
            return err
        }
        
        // 递归确保父区块
        parentHex := fmt.Sprintf("%x", b.PrevHash)
        if len(b.PrevHash) != 0 {
            if _, ok := bc.BlockByHash(parentHex); !ok {
                if err := pm.EnsureAncestors(ctx, bc, parentHex); err != nil {
                    return err
                }
            }
        }
        
        // 添加区块
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

#### 4.3.2 交易广播

**广播交易**:

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

**P2P 交易广播**:

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
    
    // 添加到交易池
    if s.mp != nil {
        _, _ = s.mp.Add(tx)
    }
    
    return p2pWriteJSON(c, p2pEnvelope{
        Type: "tx_broadcast_ack",
        Payload: mustJSON(map[string]any{"txid": txid}),
    })
}
```

### 4.4 安全机制

#### 4.4.1 连接管理

```go
type P2PServer struct {
    bc *Blockchain
    pm PeerAPI
    mp *Mempool
    
    listenAddr string
    nodeID     string
    
    maxConns    int       // 最大连接数
    maxMsgSize  int       // 最大消息大小
    sem         chan struct{}  // 连接信号量
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
    
    // 信号量限制并发连接
    s.sem = make(chan struct{}, s.maxConns)
    
    return s
}
```

#### 4.4.2 消息大小限制

```go
func (s *P2PServer) handleConn(c net.Conn) error {
    defer c.Close()
    
    // 设置读取超时
    _ = c.SetDeadline(time.Now().Add(15 * time.Second))
    
    // 读取消息（带大小限制）
    raw, err := p2pReadJSON(c, s.maxMsgSize)
    if err != nil {
        return err
    }
    
    // ...
}
```

#### 4.4.3 共识规则验证

```go
// 握手时验证共识规则
if strings.TrimSpace(hello.RulesHash) == "" || hello.RulesHash != s.bc.RulesHashHex() {
    _ = p2pWriteJSON(c, p2pEnvelope{
        Type: "error",
        Payload: mustJSON(map[string]any{"error": "rules_hash_mismatch"}),
    })
    return errors.New("rules hash mismatch")
}
```

---

## 5. 智能合约

### 5.1 VM 架构

#### 5.1.1 虚拟机结构

```go
type VM struct {
    stack     []interface{}  // 操作数栈
    callStack []int          // 调用栈
    pc        int            // 程序计数器
    code      []byte         // 字节码
    storage   map[string][]byte  // 存储空间
    gasLimit  int64          // Gas 限制
    gasUsed   int64          // 已用 Gas
}

type VMResult struct {
    Success bool
    Data    []byte
    GasUsed int64
    Error   string
}
```

#### 5.1.2 操作码定义

```go
const (
    // 控制流
    OP_NOP           = 0x00  // 无操作
    OP_JUMP          = 0xE1  // 跳转
    OP_JUMPI         = 0xE2  // 条件跳转
    OP_RETURN        = 0xF0  // 返回
    OP_CALL          = 0xE0  // 调用
    
    // 栈操作
    OP_PUSH1         = 0x01  // 压入 1 字节
    OP_PUSH2         = 0x02  // 压入 2 字节
    OP_PUSH32        = 0x20  // 压入 32 字节
    OP_DUP           = 0x30  // 复制栈顶
    OP_SWAP          = 0x31  // 交换栈顶两项
    OP_DROP          = 0x32  // 丢弃栈顶
    OP_TOALTSTACK    = 0x33  // 移到备用栈
    OP_FROMALTSTACK  = 0x34  // 从备用栈移回
    
    // 算术运算
    OP_ADD           = 0x40  // 加法
    OP_SUB           = 0x41  // 减法
    OP_MUL           = 0x42  // 乘法
    OP_DIV           = 0x43  // 除法
    OP_MOD           = 0x44  // 取模
    
    // 哈希运算
    OP_SHA256        = 0x50  // SHA256
    
    // 比较运算
    OP_EQUAL         = 0x60  // 相等
    OP_VERIFY        = 0x61  // 验证
    
    // 签名验证
    OP_CHECKSIG      = 0x70  // 验证签名
    OP_CHECKMULTISIG = 0x71  // 验证多重签名
)
```

#### 5.1.3 执行引擎

```go
func (vm *VM) Run() VMResult {
    for vm.pc < len(vm.code) {
        // Gas 检查
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

### 5.2 Gas 机制

#### 5.2.1 Gas 计算

```go
// 操作码 Gas 成本
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

// 创建 VM 时设置 Gas 限制
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

// 执行时检查 Gas
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

### 5.3 合约接口

#### 5.3.1 智能合约结构

```go
type SmartContract struct {
    Code    []byte            // 字节码
    Storage map[string][]byte // 存储空间
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

// 部署合约
func (sc *SmartContract) Deploy(gasLimit int64) VMResult {
    vm := NewVM(sc.Code, sc.Storage, gasLimit)
    return vm.Run()
}

// 调用合约
func (sc *SmartContract) Call(method string, params []interface{}, gasLimit int64) VMResult {
    vm := NewVM(sc.Code, sc.Storage, gasLimit)
    return vm.Run()
}

// 存储操作
func (sc *SmartContract) GetStorage(key string) []byte {
    return sc.Storage[key]
}

func (sc *SmartContract) SetStorage(key string, value []byte) {
    sc.Storage[key] = value
}
```

#### 5.3.2 合约数据解析

```go
func ParseContractData(data string) (string, []interface{}, error) {
    // 格式：method:param1:param2:...
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

### 5.4 合约安全

#### 5.4.1 代币合约

```go
type TokenContract struct {
    *SmartContract
    Name        string             // 代币名称
    Symbol      string             // 代币符号
    Decimals    uint8              // 小数位数
    TotalSupply uint64             // 总供应量
    Balances    map[string]uint64  // 余额映射
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

// 转账（带溢出检查）
func (tc *TokenContract) Transfer(from, to string, amount uint64) error {
    fromBalance := tc.Balances[from]
    
    // 余额检查
    if fromBalance < amount {
        return fmt.Errorf("insufficient balance")
    }
    
    // 安全减法
    tc.Balances[from] -= amount
    
    // 安全加法（检查溢出）
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

#### 5.4.2 多重签名合约

```go
type MultiSigContract struct {
    *SmartContract
    Required int        // 所需签名数
    PubKeys  []string   // 公钥列表
}

func NewMultiSigContract(required int, pubKeys []string) *MultiSigContract {
    // 验证参数
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

// 验证签名
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

## 6. AI 审计

### 6.1 智能合约分析

#### 6.1.1 安全规则

```go
type SecurityRule struct {
    Name           string  // 规则名称
    Severity       string  // 严重程度（critical/high/medium/low）
    Pattern        string  // 正则表达式模式
    Description    string  // 描述
    Recommendation string  // 修复建议
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

#### 6.1.2 分析结果

```go
type AnalysisResult struct {
    IsSafe      bool            // 是否安全
    Issues      []SecurityIssue // 发现的问题
    Summary     string          // 摘要
    SafeOpcodes []string        // 安全的操作码
    RiskScore   float64         // 风险评分（0-100）
}

type SecurityIssue struct {
    Type           string  // 问题类型
    Severity       string  // 严重程度
    Location       string  // 位置
    Description    string  // 描述
    Recommendation string  // 建议
}
```

#### 6.1.3 字节码分析

```go
type ContractAnalyzer struct {
    rules []SecurityRule
}

func NewContractAnalyzer() *ContractAnalyzer {
    rules := make([]SecurityRule, len(contractSecurityRules))
    copy(rules, contractSecurityRules)
    return &ContractAnalyzer{rules: rules}
}

// 分析字节码
func (ca *ContractAnalyzer) Analyze(bytecode string) *AnalysisResult {
    result := &AnalysisResult{
        IsSafe:      true,
        Issues:      []SecurityIssue{},
        SafeOpcodes: []string{},
    }
    
    bytecodeLower := strings.ToLower(bytecode)
    
    // 检查危险操作码
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
    
    // 检查安全操作码
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
    
    // 生成摘要
    if len(result.Issues) == 0 {
        result.Summary = "No obvious vulnerabilities detected in bytecode"
    } else {
        result.Summary = "Contract analysis complete"
    }
    
    // 风险评分阈值
    if result.RiskScore > 70 {
        result.IsSafe = false
    } else if result.RiskScore > 30 {
        result.IsSafe = false
    }
    
    return result
}
```

#### 6.1.4 源代码分析

```go
// 分析源代码
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
    
    // 应用安全规则
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
            
            // 根据严重程度增加风险分
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
    
    // 生成结果
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

#### 6.1.5 综合安全评分

```go
// 获取安全评分
func (ca *ContractAnalyzer) GetSecurityScore(bytecode, sourceCode string) float64 {
    result := ca.Analyze(bytecode)
    
    if sourceCode != "" {
        sourceResult := ca.AnalyzeSource(sourceCode)
        result.Issues = append(result.Issues, sourceResult.Issues...)
        result.RiskScore = (result.RiskScore + sourceResult.RiskScore) / 2
    }
    
    return result.RiskScore
}

// 检查常见漏洞
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

## 7. 治理与升级

### 7.1 协议版本

```go
type ProtocolVersion struct {
    Major uint64 `json:"major"`  // 主版本号（不兼容变更）
    Minor uint64 `json:"minor"`  // 次版本号（向后兼容功能）
    Patch uint64 `json:"patch"`  // 修订号（向后兼容修复）
}

type UpgradeProposal struct {
    ID          string          `json:"id"`           // 提案 ID
    Title       string          `json:"title"`        // 提案标题
    Description string          `json:"description"`  // 提案描述
    Version     ProtocolVersion `json:"version"`      // 目标版本
    Proposer    string          `json:"proposer"`     // 提案人
    Timestamp   int64           `json:"timestamp"`    // 提案时间
    Status      string          `json:"status"`       // 状态（pending/voting/passed/rejected）
}

type ProtocolUpgrade struct {
    Version        ProtocolVersion `json:"version"`
    ActivationHeight uint64        `json:"activationHeight"`  // 激活高度
    Description    string          `json:"description"`
    Features       []string        `json:"features"`
}
```

### 7.2 升级机制

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

// 验证升级
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

## 8. 监控指标

### 8.1 Prometheus 指标

NogoChain 使用 `prometheus/client_golang` 暴露关键指标：

**区块指标**:
- `nogo_block_height` - 当前区块高度
- `nogo_block_time` - 区块生成时间间隔
- `nogo_block_difficulty` - 当前难度
- `nogo_block_tx_count` - 区块交易数量

**交易指标**:
- `nogo_mempool_size` - 交易池大小
- `nogo_mempool_fee_avg` - 平均交易费用
- `nogo_transaction_count` - 总交易数

**网络指标**:
- `nogo_peer_count` - 连接节点数
- `nogo_peer_score_avg` - 平均节点评分
- `nogo_network_bytes_sent` - 发送字节数
- `nogo_network_bytes_received` - 接收字节数

**系统指标**:
- `nogo_chain_work` - 累计工作量
- `nogo_state_size` - 状态大小

### 8.2 日志系统

**结构化日志（JSON 格式）**:

```go
type LogEntry struct {
    Timestamp string `json:"timestamp"`
    Level     string `json:"level"`      // debug/info/warn/error
    Message   string `json:"message"`
    TraceID   string `json:"trace_id,omitempty"`
    
    // 上下文信息
    BlockHeight uint64 `json:"block_height,omitempty"`
    TxID        string `json:"tx_id,omitempty"`
    Peer        string `json:"peer,omitempty"`
    Duration    int64  `json:"duration_ms,omitempty"`
}
```

**日志级别**:
- `DEBUG`: 调试信息（开发环境）
- `INFO`: 一般信息（生产环境默认）
- `WARN`: 警告信息（需要注意但不影响运行）
- `ERROR`: 错误信息（需要立即处理）

**敏感信息保护**:
- 禁止打印私钥、密码、Token 等敏感信息
- 地址显示时可选择性脱敏
- 网络请求不记录完整 payload

---

## 附录

### A. 环境变量配置

| 变量名 | 描述 | 默认值 |
|--------|------|--------|
| `CHAIN_ID` | 链 ID | 1 |
| `MINER_ADDRESS` | 矿工地址 | - |
| `PEERS` | 节点列表 | - |
| `DIFFICULTY_ENABLE` | 启用难度调整 | false |
| `DIFFICULTY_TARGET_MS` | 目标出块时间（毫秒） | 15000 |
| `DIFFICULTY_WINDOW` | 难度调整窗口 | 20 |
| `MAX_BLOCK_SIZE` | 最大区块大小 | 1000000 |
| `MERKLE_ENABLE` | 启用 Merkle 树 | false |
| `BINARY_ENCODING_ENABLE` | 启用二进制编码 | false |

### B. 文件结构

```
data/
├── blocks.dat       # 区块数据
├── state.dat        # 状态数据
├── genesis_hash.txt # 创世块哈希
└── rules_hash.txt   # 共识规则哈希

keystore/
└── UTC--<address>   # 加密钱包文件

genesis.json         # 创世块配置
```

### C. API 端点

**HTTP API**:
- `GET /chain/info` - 链信息
- `GET /blocks/height/:height` - 按高度查询区块
- `GET /blocks/hash/:hash` - 按哈希查询区块
- `GET /tx/:txid` - 查询交易
- `POST /tx` - 提交交易
- `GET /balance/:address` - 查询余额
- `GET /address/:address/txs` - 地址交易历史

**WebSocket**:
- `ws://<host>/ws` - 实时事件订阅
  - 订阅类型：`new_block`, `new_tx`, `address:<address>`

---

**文档版本**: 1.0  
**最后更新**: 2026-04-01  
**维护者**: NogoChain 开发团队
