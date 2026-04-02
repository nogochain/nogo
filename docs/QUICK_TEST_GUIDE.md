# 区块浏览器 WebSocket 实时推送 - 快速测试指南

## 🚀 快速开始

### 1. 启动节点

确保节点已启动并开启了 WebSocket：

```bash
# 检查节点日志，应该看到 WebSocket 端点启动
# 类似：Listening WebSocket on /ws
```

### 2. 打开区块浏览器

在浏览器中访问：
```
http://localhost:8080/
```

### 3. 打开开发者工具

按 `F12` 打开控制台（Console）

### 4. 观察连接日志

**成功连接**：
```
Attempting WebSocket connection to: ws://localhost:8080/ws
✓ WebSocket connected
Subscribed to new_block events
```

## 📋 测试清单

### ✅ 测试 1: WebSocket 正常连接

**步骤**：
1. 打开浏览器
2. 查看控制台

**预期**：
- [ ] 显示 `✓ WebSocket connected`
- [ ] 显示 `Subscribed to new_block events`
- [ ] 无错误信息

### ✅ 测试 2: 实时接收新区块

**步骤**：
1. 保持浏览器打开
2. 等待新区块生成（或手动触发）
3. 观察控制台和页面

**预期**：
- [ ] 控制台显示 `🆕 New block event received: {height: ...}`
- [ ] 区块高度统计立即更新
- [ ] 新区块出现在表格第一行
- [ ] 显示 Toast 通知 `🆕 New block #xxx`
- [ ] 页面无跳动

### ✅ 测试 3: 断线重连

**步骤**：
1. 打开浏览器（WebSocket 已连接）
2. 停止节点
3. 观察控制台
4. 等待重连

**预期**：
- [ ] 显示 `WebSocket closed, code: 1006`
- [ ] 显示 `Scheduling WebSocket reconnect attempt 1/10 in 3000ms`
- [ ] 每 3 秒重试一次（延迟递增）
- [ ] 达到 10 次后显示降级信息

### ✅ 测试 4: 降级到轮询

**步骤**：
1. 确保 WebSocket 重连失败（节点未启动）
2. 观察控制台
3. 等待 5 秒

**预期**：
- [ ] 显示 `Max WebSocket reconnection attempts reached. Using polling fallback.`
- [ ] 每 5 秒显示 `New block detected! Height: xxx`
- [ ] 区块仍然正常更新

### ✅ 测试 5: 恢复连接

**步骤**：
1. WebSocket 已降级到轮询
2. 重新启动节点
3. 刷新浏览器页面

**预期**：
- [ ] 重新建立 WebSocket 连接
- [ ] 显示 `✓ WebSocket connected`
- [ ] 停止轮询（仅 WebSocket 工作）

## 🔍 调试技巧

### 查看 WebSocket 状态

在浏览器控制台输入：
```javascript
// 查看连接对象
console.log(wsConnection);

// 查看连接状态 (0=CONNECTING, 1=OPEN, 2=CLOSING, 3=CLOSED)
console.log('Ready State:', wsConnection?.readyState);

// 查看重连次数
console.log('Reconnect Attempts:', wsReconnectAttempts);

// 查看已显示的区块高度
console.log('Displayed Heights:', displayedBlockHeights);
```

### 手动触发重连

```javascript
// 关闭当前连接
closeWebSocket();

// 重新建立连接
initializeWebSocket();
```

### 模拟 WebSocket 消息

```javascript
// 模拟收到新区块事件
handleNewBlockEvent({
  height: 99999,
  hash: '0x1234567890abcdef',
  prevHash: '0xabcdef1234567890',
  difficultyBits: 123456,
  txCount: 10,
  addresses: ['NOGO123...', 'NOGO456...']
});
```

### 查看定时器

```javascript
// 查看轮询定时器
// 应该在控制台看到 setInterval 的 ID
```

## ⚠️ 常见问题

### Q1: WebSocket 连接失败

**现象**：
```
WebSocket closed, code: 1006
```

**原因**：
- 节点未启动 WebSocket
- 端口被防火墙阻止
- 浏览器不支持 WebSocket

**解决**：
1. 检查节点日志，确认 WebSocket 已启用
2. 检查防火墙设置
3. 等待自动降级到轮询

### Q2: 没有收到 new_block 事件

**现象**：
- WebSocket 已连接
- 但长时间没有收到事件

**原因**：
- 没有新区块生成
- 订阅失败

**解决**：
```javascript
// 手动重新订阅
subscribeToBlocks();
```

### Q3: 页面仍然跳动

**现象**：
- 新区块显示时页面跳动

**原因**：
- 可能是其他部分在重新渲染

**解决**：
1. 检查是否有其他代码在修改表格
2. 确认 `addNewBlock()` 被调用而不是 `loadStats()`

## 📊 性能监控

### 延迟测试

```javascript
// 在控制台记录事件接收时间
const startTime = Date.now();
handleNewBlockEvent({height: 99999});
const endTime = Date.now();
console.log('处理延迟:', endTime - startTime, 'ms');
// 应该 < 100ms
```

### 内存测试

```javascript
// 长时间运行后检查内存
performance.memory.usedJSHeapSize
// 应该保持稳定，无明显增长
```

### 连接稳定性

```javascript
// 记录连接断开次数
let disconnectCount = 0;
wsConnection.onclose = () => {
  disconnectCount++;
  console.log('Disconnect count:', disconnectCount);
};
```

## 🎯 验收标准

完成以下检查确保实现正确：

- [ ] WebSocket 成功连接
- [ ] 成功订阅 `new_block` 事件
- [ ] 新区块实时显示（<100ms）
- [ ] 区块增量添加到顶部
- [ ] 页面无跳动
- [ ] Toast 通知显示
- [ ] 断线自动重连（最多 10 次）
- [ ] 重连失败自动降级到轮询
- [ ] 轮询正常工作（5 秒间隔）
- [ ] 控制台日志清晰可读

## 📝 测试报告模板

```
测试日期：2026-04-02
测试人员：[姓名]
浏览器：Chrome 120 / Firefox 120 / Safari 17

测试结果：
✅ WebSocket 连接：正常
✅ 实时推送：延迟 <100ms
✅ 增量刷新：无跳动
✅ 断线重连：正常（10 次）
✅ 降级轮询：正常（5 秒）
✅ Toast 通知：正常显示

问题记录：
- 无 / [描述问题]

备注：
- [其他观察]
```

## 🔗 相关文档

- [WebSocket 实现文档](./BLOCK_EXPLORER_WEBSOCKET.md)
- [English Documentation](./BLOCK_EXPLORER_WEBSOCKET_EN.md)
- [实现总结](./IMPLEMENTATION_SUMMARY.md)

---

**版本**: v2.0  
**日期**: 2026-04-02  
**作者**: NogoChain Team
