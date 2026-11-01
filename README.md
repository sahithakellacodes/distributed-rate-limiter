# Distributed Rate Limiter

### Why we need a rate limiter
Every API request consumes system resources, and when a client sends excessive requests (whether it is a malicious bot or a misbehaving user) it can overwhelm our service, making it slower or causing it to fail. A rate limiter controls how frequently clients can send requests, helping protect the service and maintain availability for other users.

A naive solution to solve this would be to keep track of a counter for every client in memory, reset it every minute, and reject requests once the count is exceeded. While this works in case of a single API gateway instace, modern systems rely on multiple gateway instances to distribute incoming traffic. When the same client sends requests to different gateways, they all keep track of the requests in their own in-memory counters which would mean the client could exceed the intended rate limit because each gateway only sees a portion of the client's requests. This raises the need for all servers to use a shared state to keep track of request count.

### Where to store and update counters?
A common approach is to use Redis as a shared data store for maintaining and updating counters across multiple gateway instances. However, there's still a problem. Each operation (getting the counter, incrementing it, and setting the updated value) is performed independently. Concurrent execution and interleaving of these operations can result in race conditions, causing the counter to hold an incorrect value.

In order to address this issue, we can make this sequence of operations as a single operation that executes atomically. Redis supports Lua scripting, which allows us to execute multiple operations atomically, preventing other Redis commands from interleaving with the script's execution.

### Why not MULTI/EXEC and WATCH?
Lua avoids application level optimistic locking (used by WATCH) and retries which reduces number of network round trips since logic is located within Redis server. This also keeps the implementation simpler since we do not have to implement retry logic.

### Which algorithm to use?
The most basic algorithm that one can use is a **Fixed Window Counter**. This works by maintaining a fixed window size and allowing a maximum of N requests per window. The downside of such an algorithm is that a client can send a sudden burst of requests at the window boundary i.e. right before the current window expires and right after the next window starts. This would result in 2N requests in a very short span of time which is a huge spike of requests.

To combat this we can use a **Sliding Window Counter** algorithm where we approximate a continuously moving window. This uses a weighted average of the requests in previous window and current window along with the elapsed fraction of current window to decide whether to allow a request or not. While we can use a Sliding Window Log (which keeps a log of every single request and it's timestamp) for more precision, it results in a huge memory overhead, especially with a large number of clients and requests.

### API Gateway and Middlewares
What makes a rate limiter useful is an API Gateway that acts as a supporting infrastructure for the rate limiter alongside other helpful middlewares. The following are some of the additional middlewares and services that are bundled together with the rate limiter as a part of this service:

- Authentication
- Router
- Health Check
- Logger

### Testing
