# Paguu API 文档

## 启动 API 服务器

```bash
go run cmd/server/main.go
```

服务器将在 `http://localhost:8080` 上启动。

## API 端点

### 1. 创建新任务

**POST** `/api/v1/tasks`

提交一批原始问题，系统会自动进行丰富化处理和向量化。

#### 请求体

```json
{
  "raw_questions": "2. 项目引发：io.ReadAll和io.Copy的区别\n3. slice的扩容机制\n4. golang的GC",
  "source": "面试题集合",
  "metadata": {
    "company": "某公司",
    "date": "2025-10-26"
  }
}
```

#### 参数说明
- `raw_questions` (必需): 原始问题文本
- `source` (可选): 问题来源
- `metadata` (可选): 自定义元数据

#### 示例请求

```bash
curl -X POST "http://localhost:8080/api/v1/tasks" \
  -H "Content-Type: application/json" \
  -d '{
    "raw_questions": "1. Go的并发模型\n2. MySQL索引优化\n3. Redis持久化机制",
    "source": "技术面试",
    "metadata": {
      "level": "senior",
      "category": "backend"
    }
  }'
```

#### 响应示例

```json
{
  "message": "task created successfully",
  "task_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "data": {
    "task_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "source": "技术面试",
    "created_at": "2025-10-26 15:30:00"
  }
}
```

**说明**: 任务创建后会自动进入处理队列，系统会在后台进行问题丰富化、向量化和存储。

---

### 2. 获取文章列表（支持 tag 筛选）

**GET** `/api/v1/articles`

#### 查询参数
- `page` (可选): 页码，默认 1
- `page_size` (可选): 每页数量，默认 20，最大 100
- `tags[]` (可选): Tag 筛选，可多选

#### 示例请求

```bash
# 获取所有文章（第1页，每页20条）
curl "http://localhost:8080/api/v1/articles"

# 获取第2页，每页10条
curl "http://localhost:8080/api/v1/articles?page=2&page_size=10"

# 筛选包含 "Go" tag 的文章
curl "http://localhost:8080/api/v1/articles?tags[]=Go"

# 筛选同时包含 "Go" 和 "MySQL" tags 的文章
curl "http://localhost:8080/api/v1/articles?tags[]=Go&tags[]=MySQL"
```

#### 响应示例

```json
{
  "data": [
    {
      "id": 123,
      "original_question": "什么是 Go 的 GC？",
      "detailed_question": "详细描述 Go 语言的垃圾回收机制...",
      "concise_answer": "Go 使用三色标记清除算法...",
      "tags": ["Go", "GC", "内存管理"],
      "created_at": "2025-10-26 14:30:00"
    }
  ],
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total": 150,
    "total_page": 8
  }
}
```

---

### 3. 向量相似度搜索

**POST** `/api/v1/articles/search`

根据查询文本找到语义相似的文章。

#### 请求体

```json
{
  "query": "如何优化 MySQL 查询性能",
  "limit": 10
}
```

#### 参数说明
- `query` (必需): 查询文本
- `limit` (必需): 返回结果数量，范围 1-100

#### 示例请求

```bash
curl -X POST "http://localhost:8080/api/v1/articles/search" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "Go 语言的并发模型",
    "limit": 5
  }'
```

#### 响应示例

```json
{
  "data": [
    {
      "id": 45,
      "original_question": "Go 的 goroutine 和 channel",
      "detailed_question": "详细解释 goroutine 的工作原理...",
      "concise_answer": "Goroutine 是 Go 的轻量级线程...",
      "tags": ["Go", "并发", "Goroutine"],
      "created_at": "2025-10-25 10:20:00",
      "similarity": 0.92
    },
    {
      "id": 78,
      "original_question": "CSP 并发模型",
      "detailed_question": "Go 使用的 CSP 模型是什么...",
      "concise_answer": "CSP 是通过通信来共享内存...",
      "tags": ["Go", "并发", "CSP"],
      "created_at": "2025-10-24 16:45:00",
      "similarity": 0.87
    }
  ],
  "query": "Go 语言的并发模型"
}
```

**注意**: `similarity` 值范围 0-1，越接近 1 表示越相似。

---

### 4. 获取单篇文章详情

**GET** `/api/v1/articles/:id`

#### 示例请求

```bash
curl "http://localhost:8080/api/v1/articles/123"
```

#### 响应示例

```json
{
  "data": {
    "id": 123,
    "original_question": "什么是 Go 的 GC？",
    "detailed_question": "详细描述 Go 语言的垃圾回收机制...",
    "concise_answer": "Go 使用三色标记清除算法...",
    "tags": ["Go", "GC", "内存管理"],
    "created_at": "2025-10-26 14:30:00"
  }
}
```

---

### 5. 获取所有 Tag 列表

**GET** `/api/v1/tags`

获取系统中所有不重复的 tag，用于前端筛选器。

#### 示例请求

```bash
curl "http://localhost:8080/api/v1/tags"
```

#### 响应示例

```json
{
  "data": [
    "并发",
    "内存管理",
    "数据库",
    "Go",
    "MySQL",
    "性能优化"
  ]
}
```

---

## 错误响应

所有端点在出错时返回类似格式：

```json
{
  "error": "错误描述信息"
}
```

常见 HTTP 状态码：
- `200 OK`: 成功
- `201 Created`: 创建成功
- `400 Bad Request`: 请求参数错误
- `404 Not Found`: 资源不存在
- `500 Internal Server Error`: 服务器内部错误

---

## 前端集成示例

### 场景 1: 创建新任务

```javascript
// 用户提交一批问题
const rawQuestions = `
1. Go 的 GC 机制
2. MySQL 的索引结构
3. Redis 的数据类型
`;

fetch('http://localhost:8080/api/v1/tasks', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    raw_questions: rawQuestions,
    source: '面试题集合',
    metadata: {
      submitter: 'user123',
      category: 'backend'
    }
  })
})
  .then(res => res.json())
  .then(data => {
    console.log('任务已创建:', data.task_id);
    console.log('任务将在后台自动处理');
  });
```

### 场景 2: 展示文章列表（按 ID 降序）

```javascript
// 获取第一页数据
fetch('http://localhost:8080/api/v1/articles?page=1&page_size=20')
  .then(res => res.json())
  .then(data => {
    console.log('文章列表:', data.data);
    console.log('总共', data.pagination.total, '篇文章');
  });
```

### 场景 3: 点击 Tag 筛选

```javascript
// 用户点击 "Go" 标签
const selectedTags = ['Go', 'MySQL'];
const query = selectedTags.map(tag => `tags[]=${encodeURIComponent(tag)}`).join('&');

fetch(`http://localhost:8080/api/v1/articles?${query}`)
  .then(res => res.json())
  .then(data => {
    console.log('筛选后的文章:', data.data);
  });
```

### 场景 4: 向量搜索

```javascript
// 用户输入搜索关键词
const searchQuery = "如何优化数据库查询";

fetch('http://localhost:8080/api/v1/articles/search', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    query: searchQuery,
    limit: 10
  })
})
  .then(res => res.json())
  .then(data => {
    console.log('相似文章:', data.data);
    // 可以根据 similarity 值显示相似度百分比
    data.data.forEach(article => {
      console.log(`${article.original_question} (${(article.similarity * 100).toFixed(1)}% 相似)`);
    });
  });
```

---

## 配置说明

确保 `configs/config.yaml` 中配置了正确的参数：

```yaml
gemini:
  api_key: "your-gemini-api-key"
  embedding_model: "gemini-embedding-001"

database:
  dsn: "host=localhost user=myuser password=mypassword dbname=mydb port=5432 sslmode=disable TimeZone=Asia/Shanghai"
```

或通过环境变量设置：
- `GEMINI_API_KEY`
- `DATABASE_DSN`
