# Block Explorer WebSocket Real-time Push - Quick Test Guide

## 🚀 Quick Start

### 1. Start Node

Ensure the node is started and WebSocket is enabled:

```bash
# Check node logs, should see WebSocket endpoint startup
# Similar to: Listening WebSocket on /ws
```

### 2. Open Block Explorer

Access in browser:
```
http://localhost:8080/
```

### 3. Open Developer Tools

Press `F12` to open Console

### 4. Observe Connection Logs

**Successful Connection**:
```
Attempting WebSocket connection to: ws://localhost:8080/ws
✓ WebSocket connected
Subscribed to new_block events
```

## 📋 Test Checklist

### ✅ Test 1: WebSocket Normal Connection

**Steps**:
1. Open browser
2. Check console

**Expected**:
- [ ] Shows `✓ WebSocket connected`
- [ ] Shows `Subscribed to new_block events`
- [ ] No error messages

### ✅ Test 2: Real-time New Block Reception

**Steps**:
1. Keep browser open
2. Wait for new block generation (or manually trigger)
3. Observe console and page

**Expected**:
- [ ] Console shows `🆕 New block event received: {height: ...}`
- [ ] Block height statistics update immediately
- [ ] New block appears in first row of table
- [ ] Toast notification shows `🆕 New block #xxx`
- [ ] No page jumping

### ✅ Test 3: Disconnection & Reconnection

**Steps**:
1. Open browser (WebSocket connected)
2. Stop node
3. Observe console
4. Wait for reconnection

**Expected**:
- [ ] Shows `WebSocket closed, code: 1006`
- [ ] Shows `Scheduling WebSocket reconnect attempt 1/10 in 3000ms`
- [ ] Retries every 3 seconds (increasing delay)
- [ ] Shows fallback message after 10 attempts

### ✅ Test 4: Fallback to Polling

**Steps**:
1. Ensure WebSocket reconnection failed (node not started)
2. Observe console
3. Wait 5 seconds

**Expected**:
- [ ] Shows `Max WebSocket reconnection attempts reached. Using polling fallback.`
- [ ] Shows `New block detected! Height: xxx` every 5 seconds
- [ ] Blocks still update normally

### ✅ Test 5: Connection Recovery

**Steps**:
1. WebSocket has fallen back to polling
2. Restart node
3. Refresh browser page

**Expected**:
- [ ] Re-establishes WebSocket connection
- [ ] Shows `✓ WebSocket connected`
- [ ] Stops polling (only WebSocket active)

## 🔍 Debugging Tips

### Check WebSocket Status

Enter in browser console:
```javascript
// Check connection object
console.log(wsConnection);

// Check connection status (0=CONNECTING, 1=OPEN, 2=CLOSING, 3=CLOSED)
console.log('Ready State:', wsConnection?.readyState);

// Check reconnection attempts
console.log('Reconnect Attempts:', wsReconnectAttempts);

// Check displayed block heights
console.log('Displayed Heights:', displayedBlockHeights);
```

### Manually Trigger Reconnection

```javascript
// Close current connection
closeWebSocket();

// Re-establish connection
initializeWebSocket();
```

### Simulate WebSocket Message

```javascript
// Simulate receiving new block event
handleNewBlockEvent({
  height: 99999,
  hash: '0x1234567890abcdef',
  prevHash: '0xabcdef1234567890',
  difficultyBits: 123456,
  txCount: 10,
  addresses: ['NOGO123...', 'NOGO456...']
});
```

### Check Timers

```javascript
// Check polling timer
// Should see setInterval ID in console
```

## ⚠️ Common Issues

### Q1: WebSocket Connection Failure

**Symptoms**:
```
WebSocket closed, code: 1006
```

**Causes**:
- Node WebSocket not started
- Port blocked by firewall
- Browser does not support WebSocket

**Solutions**:
1. Check node logs, confirm WebSocket is enabled
2. Check firewall settings
3. Wait for auto-fallback to polling

### Q2: No new_block Events Received

**Symptoms**:
- WebSocket connected
- But no events received for long time

**Causes**:
- No new blocks being generated
- Subscription failed

**Solution**:
```javascript
// Manually resubscribe
subscribeToBlocks();
```

### Q3: Page Still Jumps

**Symptoms**:
- Page jumps when new block displays

**Causes**:
- Other parts may be re-rendering

**Solutions**:
1. Check if other code is modifying the table
2. Confirm `addNewBlock()` is called instead of `loadStats()`

## 📊 Performance Monitoring

### Latency Test

```javascript
// Record event reception time in console
const startTime = Date.now();
handleNewBlockEvent({height: 99999});
const endTime = Date.now();
console.log('Processing latency:', endTime - startTime, 'ms');
// Should be < 100ms
```

### Memory Test

```javascript
// Check memory after long-running operation
performance.memory.usedJSHeapSize
// Should remain stable, no obvious growth
```

### Connection Stability

```javascript
// Record disconnection count
let disconnectCount = 0;
wsConnection.onclose = () => {
  disconnectCount++;
  console.log('Disconnect count:', disconnectCount);
};
```

## 🎯 Acceptance Criteria

Complete the following checks to ensure correct implementation:

- [ ] WebSocket connects successfully
- [ ] Successfully subscribes to `new_block` events
- [ ] New blocks display in real-time (<100ms)
- [ ] Blocks incrementally added to top
- [ ] No page jumping
- [ ] Toast notifications display
- [ ] Auto-reconnection on disconnect (max 10 attempts)
- [ ] Auto-fallback to polling after reconnection failures
- [ ] Polling works normally (5 second interval)
- [ ] Console logs are clear and readable

## 📝 Test Report Template

```
Test Date: 2026-04-02
Tester: [Name]
Browser: Chrome 120 / Firefox 120 / Safari 17

Test Results:
✅ WebSocket Connection: Normal
✅ Real-time Push: Latency <100ms
✅ Incremental Refresh: No jumping
✅ Disconnection Reconnection: Normal (10 attempts)
✅ Fallback Polling: Normal (5 seconds)
✅ Toast Notifications: Normal display

Issue Record:
- None / [Describe issues]

Remarks:
- [Other observations]
```

## 🔗 Related Documentation

- [WebSocket Implementation Guide](./BLOCK_EXPLORER_WEBSOCKET.md)
- [English Documentation](./BLOCK_EXPLORER_WEBSOCKET_EN.md)
- [Implementation Summary](./IMPLEMENTATION_SUMMARY.md)
- [Implementation Summary English](./IMPLEMENTATION_SUMMARY_EN.md)

---

**Version**: v2.0  
**Date**: 2026-04-02  
**Author**: NogoChain Team
