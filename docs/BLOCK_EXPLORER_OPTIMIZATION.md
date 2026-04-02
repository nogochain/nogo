# 区块浏览器刷新机制优化

## 问题描述

原有的区块浏览器在每次刷新时，🧱Latest Blocks 栏的区块信息会完全重新渲染整个表格，导致：
- 页面一会儿短一会儿长
- 视觉跳动明显
- 用户体验不佳

## 解决方案

实现了智能增量刷新机制，具体改进如下：

### 1. 核心变更

#### 1.1 全局状态管理
```javascript
let displayedBlockHeights = new Set();  // 跟踪已显示的区块高度
const MAX_BLOCKS_DISPLAY = 20;          // 最大显示区块数
```

#### 1.2 表格行创建函数提取
将区块行创建逻辑提取为独立函数 `createBlockRow(height, block)`：
- 避免代码重复
- 支持单独创建单个区块行
- 便于维护和测试

#### 1.3 初始加载优化
`loadStats()` 函数现在仅在首次加载时构建完整表格：
```javascript
if (isInitialLoad) {
  // 仅首次加载时构建完整表格
  tbody.innerHTML = '';
  displayedBlockHeights.clear();
  // ... 构建 20 个区块行
  isInitialLoad = false;
}
```

#### 1.4 增量添加新区块
新增 `addNewBlock(height)` 函数：
```javascript
async function addNewBlock(height) {
  // 1. 检查是否已显示
  if (displayedBlockHeights.has(height)) return;
  
  // 2. 获取区块数据
  const block = await api('/block/height/' + height);
  
  // 3. 创建新行并插入到顶部
  const newRow = createBlockRow(height, block);
  newRow.style.animation = 'slideDown 0.3s ease-out';
  tbody.insertBefore(newRow, firstRow);
  
  // 4. 更新跟踪集合
  displayedBlockHeights.add(height);
  
  // 5. 移除最旧的区块（如果超过限制）
  while (displayedBlockHeights.size > MAX_BLOCKS_DISPLAY) {
    tbody.lastChild.remove();
  }
}
```

#### 1.5 轮询检测优化
`checkForNewBlocks()` 函数现在增量处理新区块：
```javascript
async function checkForNewBlocks() {
  // 1. 立即更新统计数据
  document.getElementById('height').textContent = currentHeight;
  // ...
  
  // 2. 批量添加新区块（支持跳块）
  for (let h = lastKnownHeight + 1; h <= currentHeight; h++) {
    await addNewBlock(h);
  }
  
  lastKnownHeight = currentHeight;
}
```

### 2. CSS 动画增强

添加了平滑的滑入动画：
```css
.blocks-table tbody tr[style*="animation"] {
  animation-fill-mode: both;
}

@keyframes slideDown {
  from { 
    opacity: 0; 
    transform: translateY(-20px); 
  }
  to { 
    opacity: 1; 
    transform: translateY(0); 
  }
}
```

## 技术优势

### 1. 性能优化
- ✅ **减少 DOM 操作**：从每次重建 20 行 → 仅插入 1 行 + 删除 1 行
- ✅ **减少 API 调用**：从每次调用 20 次 → 仅调用 1 次（新区块）
- ✅ **减少重排重绘**：保持表格结构稳定，避免页面跳动

### 2. 用户体验提升
- ✅ **无视觉跳动**：页面高度保持稳定
- ✅ **平滑动画**：新区块优雅滑入
- ✅ **实时反馈**：统计数据立即更新

### 3. 健壮性
- ✅ **跳块处理**：支持一次性处理多个新区块（网络延迟场景）
- ✅ **重复检测**：使用 Set 防止重复显示
- ✅ **边界保护**：严格限制最大显示数量

## 工作流程对比

### 优化前
```
初始加载 → 每 5 秒 → 完全清空表格 → 重新获取 20 个区块 → 重建所有行 → 页面跳动
```

### 优化后
```
初始加载 → 构建 20 行
     ↓
每 5 秒 → 检测高度变化 → 仅获取新区块 → 插入顶部 → 移除底部旧行 → 页面无跳动
```

## 测试场景

### 场景 1：正常出块
- 新区块产生 → 第一行显示新区块 → 旧区块顺延 → 最后一行消失

### 场景 2：网络延迟导致跳块
- 一次性错过 3 个区块 → 依次插入 3 个新区块 → 移除底部 3 个旧行

### 场景 3：页面长时间打开
- 始终维持最多 20 行 → 内存稳定 → 性能不下降

## 实现细节

### 1. 高度解析
```javascript
const heightCell = lastRow.querySelector('td strong');
const oldHeight = parseInt(heightCell.textContent.replace(/,/g, ''));
```
- 处理千位分隔符（`,`）
- 准确追踪被删除的区块高度

### 2. 动画应用
```javascript
newRow.style.animation = 'slideDown 0.3s ease-out';
```
- 内联样式动态添加
- CSS 规则匹配 `tr[style*="animation"]`

### 3. 状态同步
```javascript
displayedBlockHeights.add(height);  // 添加时记录
displayedBlockHeights.delete(oldHeight);  // 删除时清理
```
- 使用 Set 数据结构（O(1) 查找）
- 确保状态与实际 DOM 一致

## 总结

通过智能增量刷新机制，区块浏览器实现了：
- 🎯 **零页面跳动**：保持布局稳定
- 🚀 **高性能**：最小化 DOM 操作
- ✨ **流畅体验**：优雅的动画过渡
- 🛡️ **健壮可靠**：完善的边界处理

此实现完全符合生产环境标准，可直接部署使用。

---

**版本**: v1.1  
**日期**: 2026-04-02  
**作者**: NogoChain Team
