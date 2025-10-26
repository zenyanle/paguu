# Paguu API 使用示例

## 完整工作流程示例

### 步骤 1: 提交新问题批次

```bash
curl -X POST "http://localhost:8080/api/v1/tasks" \
  -H "Content-Type: application/json" \
  -d '{
    "raw_questions": "1. Go 的 GC 机制\n2. MySQL 索引优化\n3. Redis 持久化",
    "source": "2025年面试题",
    "metadata": {
      "level": "senior",
      "category": "backend"
    }
  }'
```

**响应**:
```json
{
  "message": "task created successfully",
  "task_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "data": {
    "task_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "source": "2025年面试题",
    "created_at": "2025-10-26 15:30:00"
  }
}
```

> **说明**: 任务提交后，系统会在后台自动：
> 1. 使用 LLM 丰富化问题（生成详细问题和简洁答案）
> 2. 生成向量嵌入
> 3. 存储到数据库
> 4. 自动去重（如果相似度 > 0.95）

---

### 步骤 2: 查看所有文章（按 ID 降序）

```bash
curl "http://localhost:8080/api/v1/articles?page=1&page_size=20"
```

**响应**:
```json
{
  "data": [
    {
      "id": 456,
      "original_question": "Redis 持久化",
      "detailed_question": "Redis 的 RDB 和 AOF 持久化机制有什么区别？各自的优缺点是什么？",
      "concise_answer": "RDB 是快照持久化，性能好但可能丢失数据；AOF 是日志持久化，数据安全性高但性能稍差。",
      "tags": ["Redis", "持久化", "数据库"],
      "created_at": "2025-10-26 15:31:05"
    },
    {
      "id": 455,
      "original_question": "MySQL 索引优化",
      "detailed_question": "如何优化 MySQL 的索引使用？什么情况下索引会失效？",
      "concise_answer": "遵循最左前缀原则，避免在索引列上使用函数，注意索引覆盖和索引下推。",
      "tags": ["MySQL", "索引", "性能优化"],
      "created_at": "2025-10-26 15:31:03"
    },
    {
      "id": 454,
      "original_question": "Go 的 GC 机制",
      "detailed_question": "Go 语言的垃圾回收机制是如何工作的？三色标记算法是什么？",
      "concise_answer": "Go 使用三色标记清除算法，通过并发标记和写屏障实现低延迟的垃圾回收。",
      "tags": ["Go", "GC", "内存管理"],
      "created_at": "2025-10-26 15:31:00"
    }
  ],
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total": 3,
    "total_page": 1
  }
}
```

---

### 步骤 3: 按 Tag 筛选文章

#### 筛选包含 "Go" 标签的文章

```bash
curl "http://localhost:8080/api/v1/articles?tags[]=Go"
```

#### 筛选同时包含 "Go" 和 "内存管理" 的文章

```bash
curl "http://localhost:8080/api/v1/articles?tags[]=Go&tags[]=内存管理"
```

**响应**:
```json
{
  "data": [
    {
      "id": 454,
      "original_question": "Go 的 GC 机制",
      "detailed_question": "Go 语言的垃圾回收机制是如何工作的？三色标记算法是什么？",
      "concise_answer": "Go 使用三色标记清除算法，通过并发标记和写屏障实现低延迟的垃圾回收。",
      "tags": ["Go", "GC", "内存管理"],
      "created_at": "2025-10-26 15:31:00"
    }
  ],
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total": 1,
    "total_page": 1
  }
}
```

---

### 步骤 4: 向量相似度搜索

#### 场景：用户想找关于"数据库性能优化"的相关问题

```bash
curl -X POST "http://localhost:8080/api/v1/articles/search" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "如何提升数据库查询性能",
    "limit": 5
  }'
```

**响应**:
```json
{
  "data": [
    {
      "id": 455,
      "original_question": "MySQL 索引优化",
      "detailed_question": "如何优化 MySQL 的索引使用？什么情况下索引会失效？",
      "concise_answer": "遵循最左前缀原则，避免在索引列上使用函数，注意索引覆盖和索引下推。",
      "tags": ["MySQL", "索引", "性能优化"],
      "created_at": "2025-10-26 15:31:03",
      "similarity": 0.89
    },
    {
      "id": 456,
      "original_question": "Redis 持久化",
      "detailed_question": "Redis 的 RDB 和 AOF 持久化机制有什么区别？各自的优缺点是什么？",
      "concise_answer": "RDB 是快照持久化，性能好但可能丢失数据；AOF 是日志持久化，数据安全性高但性能稍差。",
      "tags": ["Redis", "持久化", "数据库"],
      "created_at": "2025-10-26 15:31:05",
      "similarity": 0.72
    }
  ],
  "query": "如何提升数据库查询性能"
}
```

> **说明**: `similarity` 值范围 0-1，越接近 1 表示语义越相似。

---

### 步骤 5: 获取所有可用标签

```bash
curl "http://localhost:8080/api/v1/tags"
```

**响应**:
```json
{
  "data": [
    "GC",
    "Go",
    "MySQL",
    "Redis",
    "内存管理",
    "持久化",
    "数据库",
    "性能优化",
    "索引"
  ]
}
```

---

## 实际应用场景

### 场景 1: 面试题库管理

1. **导入题目**：使用 `/api/v1/tasks` 批量导入面试题
2. **自动整理**：系统自动丰富化问题，生成详细描述和答案
3. **智能去重**：相似度 > 0.95 的问题自动合并
4. **标签管理**：自动提取标签，方便分类查找

### 场景 2: 知识库搜索

1. **关键词搜索**：用户输入"Go 并发"
2. **语义理解**：通过向量搜索找到所有相关问题
3. **相似度排序**：按相似度降序展示结果
4. **快速定位**：即使用词不同，也能找到相关内容

### 场景 3: 个性化推荐

1. **用户查看某个问题**：如"MySQL 索引优化"
2. **找相似问题**：使用该问题的向量搜索相似内容
3. **推荐列表**：展示"你可能还想了解..."
4. **关联学习**：帮助用户系统学习相关知识点

---

## 技术特性

✅ **自动化处理**：问题提交后全自动丰富化和向量化  
✅ **智能去重**：基于向量相似度自动合并重复问题  
✅ **语义搜索**：支持自然语言查询，理解问题意图  
✅ **标签系统**：自动提取标签，支持多标签筛选  
✅ **高性能**：使用 PostgreSQL + pgvector HNSW 索引  
✅ **并发控制**：最多 3 个任务并发处理，避免资源耗尽

---

## 性能建议

- **批量导入**：每次提交 10-50 个问题为宜
- **向量搜索**：limit 建议不超过 50
- **分页查询**：page_size 建议 20-50
- **标签筛选**：支持多标签，但不宜超过 5 个
