package queue

// luaLease atomically acquires a lease if not already held.
// KEYS[1] = lease key, ARGV[1] = nodeID, ARGV[2] = TTL seconds
// Returns 1 if acquired, 0 if already held by someone else.
const luaLease = `
local cur = redis.call('GET', KEYS[1])
if cur == false then
  redis.call('SET', KEYS[1], ARGV[1], 'EX', tonumber(ARGV[2]))
  return 1
end
if cur == ARGV[1] then
  redis.call('EXPIRE', KEYS[1], tonumber(ARGV[2]))
  return 1
end
return 0
`

// luaRenew atomically renews a lease only if still held by the same node.
// KEYS[1] = lease key, ARGV[1] = nodeID, ARGV[2] = TTL seconds
// Returns 1 if renewed, 0 if not held by this node.
const luaRenew = `
local cur = redis.call('GET', KEYS[1])
if cur == ARGV[1] then
  redis.call('EXPIRE', KEYS[1], tonumber(ARGV[2]))
  return 1
end
return 0
`

// luaComplete atomically releases a lease and removes the subtask from the
// task's pending set. Only releases if this node holds the lease.
// KEYS[1] = lease key, KEYS[2] = pending set key
// ARGV[1] = nodeID, ARGV[2] = subtask_id
// Returns 1 on success, 0 if lease not held.
const luaComplete = `
local cur = redis.call('GET', KEYS[1])
if cur ~= ARGV[1] then
  return 0
end
redis.call('DEL', KEYS[1])
redis.call('SREM', KEYS[2], ARGV[2])
return 1
`

// luaRelease releases a lease but deliberately keeps the subtask in the
// task's pending set. The caller can then requeue it without creating a
// window in which the stage is falsely considered complete.
const luaRelease = `
local cur = redis.call('GET', KEYS[1])
if cur ~= ARGV[1] then
  return 0
end
redis.call('DEL', KEYS[1])
return 1
`
