# **SQLite Agent Brain Architecture**

This document outlines the chosen architecture for our local, machine-native agent memory: **The SQLite Vector-Graph**.

By leveraging SQLite, we achieve the strict relational mapping of a Graph Database and the semantic fuzzy recall of a Vector Database, all contained within a single local file (brain.sqlite). This requires zero background servers, no Docker containers, and allows the agent to interact using robust, native CLI tools.

## **1\. The Database Schema**

The agent initializes the .sqlite file with a schema explicitly designed for both graph traversal and vector search (utilizing the sqlite-vec extension).

### **Nodes Table**

Stores the atomic memories, concepts, and resolutions.

CREATE TABLE nodes (  
    id TEXT PRIMARY KEY,  
    type TEXT NOT NULL,      \-- e.g., 'concept', 'bug', 'architecture\_decision'  
    metadata JSON,           \-- Stores the actual text/content and tags  
    created\_at DATETIME DEFAULT CURRENT\_TIMESTAMP  
);

### **Edges Table**

Stores the relationships (synapses) between nodes.

CREATE TABLE edges (  
    source\_id TEXT,  
    target\_id TEXT,  
    relation TEXT,           \-- e.g., 'blocks', 'implements', 'relates\_to'  
    FOREIGN KEY(source\_id) REFERENCES nodes(id),  
    FOREIGN KEY(target\_id) REFERENCES nodes(id)  
);

### **Vector Table (sqlite-vec)**

Stores the semantic embeddings for fuzzy recall.

CREATE VIRTUAL TABLE vec\_nodes USING vec0(  
    node\_id TEXT,  
    embedding float\[1536\]    \-- Assuming standard 1536-dimensional embeddings  
);

## **2\. Agent Access Patterns**

The agent queries this structure using standard SQL (often wrapped in a CLI tool like agent-brain to handle the embedding math automatically).

### **A. Multi-Hop Graph Traversal**

When the agent needs to find the exact dependencies of a known issue:

SELECT metadata FROM nodes   
WHERE id IN (  
    SELECT target\_id FROM edges WHERE source\_id \= 'bug\_042'  
);

### **B. Semantic Search**

When the agent faces a new problem and needs to find conceptually similar past solutions:

SELECT node\_id, distance   
FROM vec\_nodes   
WHERE embedding MATCH '\[...\]' \-- Query embedding vector  
ORDER BY distance   
LIMIT 3;

### **C. The Hybrid Workflow**

This is the ultimate agent capability:

1. **Fuzzy Search:** The agent uses semantic search to find a node related to "Authentication".  
2. **Contextual Expansion:** The agent then uses a Graph Query (JOIN on the edges table) to pull all nodes strictly connected to that Authentication node, ensuring it doesn't hallucinate missing dependencies.

## **3\. Why This Approach Wins**

* **Zero Infrastructure Overhead:** No Kubernetes, no graph database servers to maintain. It's just a file.  
* **ACID Compliant:** Safe concurrent reads/writes if multiple agents (or you and the agent) are working simultaneously.  
* **Ultimate Portability:** The entire memory network can be committed to Git (using tools like sqlite3 .dump) or shared easily between team members.