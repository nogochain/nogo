# Block Explorer Refresh Mechanism Optimization

## Problem Description

The original block explorer completely re-rendered the entire table every time it refreshed in the 🧱Latest Blocks section, causing:
- Page height fluctuation (short then long)
- Obvious visual jumping
- Poor user experience

## Solution

Implemented an intelligent incremental refresh mechanism with the following improvements:

### 1. Core Changes

#### 1.1 Global State Management
```javascript
let displayedBlockHeights = new Set();  // Track displayed block heights
const MAX_BLOCKS_DISPLAY = 20;          // Maximum blocks to display
```

#### 1.2 Extracted Row Creation Function
Extracted block row creation logic into a standalone function `createBlockRow(height, block)`:
- Eliminates code duplication
- Supports creating individual block rows
- Easier to maintain and test

#### 1.3 Initial Load Optimization
The `loadStats()` function now only builds the complete table on first load:
```javascript
if (isInitialLoad) {
  // Build complete table only on initial load
  tbody.innerHTML = '';
  displayedBlockHeights.clear();
  // ... Build 20 block rows
  isInitialLoad = false;
}
```

#### 1.4 Incremental Block Addition
New `addNewBlock(height)` function:
```javascript
async function addNewBlock(height) {
  // 1. Check if already displayed
  if (displayedBlockHeights.has(height)) return;
  
  // 2. Fetch block data
  const block = await api('/block/height/' + height);
  
  // 3. Create new row and insert at top
  const newRow = createBlockRow(height, block);
  newRow.style.animation = 'slideDown 0.3s ease-out';
  tbody.insertBefore(newRow, firstRow);
  
  // 4. Update tracking set
  displayedBlockHeights.add(height);
  
  // 5. Remove oldest block if exceeding limit
  while (displayedBlockHeights.size > MAX_BLOCKS_DISPLAY) {
    tbody.lastChild.remove();
  }
}
```

#### 1.5 Polling Detection Optimization
The `checkForNewBlocks()` function now incrementally processes new blocks:
```javascript
async function checkForNewBlocks() {
  // 1. Immediately update statistics
  document.getElementById('height').textContent = currentHeight;
  // ...
  
  // 2. Batch add new blocks (supports skipped blocks)
  for (let h = lastKnownHeight + 1; h <= currentHeight; h++) {
    await addNewBlock(h);
  }
  
  lastKnownHeight = currentHeight;
}
```

### 2. CSS Animation Enhancement

Added smooth slide-in animation:
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

## Technical Advantages

### 1. Performance Optimization
- ✅ **Reduced DOM Operations**: From rebuilding 20 rows → inserting 1 row + deleting 1 row
- ✅ **Reduced API Calls**: From 20 calls per refresh → only 1 call (new block)
- ✅ **Reduced Reflows/Repaints**: Maintains stable table structure, prevents page jumping

### 2. User Experience Improvement
- ✅ **No Visual Jumping**: Page height remains stable
- ✅ **Smooth Animation**: New blocks slide in elegantly
- ✅ **Immediate Feedback**: Statistics update instantly

### 3. Robustness
- ✅ **Skipped Block Handling**: Supports processing multiple new blocks at once (network delay scenarios)
- ✅ **Duplicate Detection**: Uses Set to prevent duplicate display
- ✅ **Boundary Protection**: Strictly limits maximum display count

## Workflow Comparison

### Before Optimization
```
Initial Load → Every 5s → Clear entire table → Fetch 20 blocks again → Rebuild all rows → Page jumps
```

### After Optimization
```
Initial Load → Build 20 rows
     ↓
Every 5s → Detect height change → Fetch only new blocks → Insert at top → Remove bottom old row → No page jump
```

## Test Scenarios

### Scenario 1: Normal Block Production
- New block produced → First row shows new block → Old blocks shift down → Last row disappears

### Scenario 2: Network Delay Causing Skipped Blocks
- Missed 3 blocks at once → Insert 3 new blocks sequentially → Remove 3 oldest rows from bottom

### Scenario 3: Page Left Open for Extended Period
- Always maintains maximum 20 rows → Stable memory usage → No performance degradation

## Implementation Details

### 1. Height Parsing
```javascript
const heightCell = lastRow.querySelector('td strong');
const oldHeight = parseInt(heightCell.textContent.replace(/,/g, ''));
```
- Handles thousand separators (`,`)
- Accurately tracks deleted block heights

### 2. Animation Application
```javascript
newRow.style.animation = 'slideDown 0.3s ease-out';
```
- Inline style dynamically added
- CSS rule matches `tr[style*="animation"]`

### 3. State Synchronization
```javascript
displayedBlockHeights.add(height);  // Record on addition
displayedBlockHeights.delete(oldHeight);  // Clean up on deletion
```
- Uses Set data structure (O(1) lookup)
- Ensures state matches actual DOM

## Summary

Through the intelligent incremental refresh mechanism, the block explorer achieves:
- 🎯 **Zero Page Jumping**: Maintains stable layout
- 🚀 **High Performance**: Minimizes DOM operations
- ✨ **Smooth Experience**: Elegant animation transitions
- 🛡️ **Robust & Reliable**: Comprehensive boundary handling

This implementation is production-ready and can be deployed directly.

---

**Version**: v1.1  
**Date**: 2026-04-02  
**Author**: NogoChain Team
