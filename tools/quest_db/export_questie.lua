-- Run from a pinned QuestieDB checkout: luajit export_questie.lua ids.txt
local idsPath = assert(arg[1], "quest ID file required")
local client = dofile("emulator/client.lua")
local emulator = dofile("emulator/metadata.lua")
client.install({ expansion = "Forever", locale = "enUS" })
local db = emulator.loadAddon("QuestieDB.toc", "QuestieDB")
assert(db.flavor.name == "Forever", "QuestieDB selected the wrong flavor")

local function quote(value)
  return '"' .. value:gsub('[%z\1-\31\\"]', function(char)
    local escapes = { ['\\'] = '\\\\', ['"'] = '\\"', ['\n'] = '\\n',
                      ['\r'] = '\\r', ['\t'] = '\\t' }
    return escapes[char] or string.format('\\u%04x', string.byte(char))
  end) .. '"'
end

local function encode(value)
  local kind = type(value)
  if kind == "nil" then return "null" end
  if kind == "string" then return quote(value) end
  if kind == "number" or kind == "boolean" then return tostring(value) end
  assert(kind == "table", "unsupported JSON value: " .. kind)
  local count, maximum, array = 0, 0, true
  for key in pairs(value) do
    count = count + 1
    if type(key) ~= "number" or key < 1 or key % 1 ~= 0 then array = false
    else maximum = math.max(maximum, key) end
  end
  if array and count == maximum then
    local parts = {}
    for index = 1, maximum do parts[index] = encode(value[index]) end
    return "[" .. table.concat(parts, ",") .. "]"
  end
  local keys, parts = {}, {}
  for key in pairs(value) do keys[#keys + 1] = key end
  table.sort(keys, function(a, b) return tostring(a) < tostring(b) end)
  for _, key in ipairs(keys) do
    parts[#parts + 1] = quote(tostring(key)) .. ":" .. encode(value[key])
  end
  return "{" .. table.concat(parts, ",") .. "}"
end

local providers = {
  { db.Npc, "npc" }, { db.Object, "object" }, { db.Item, "item" },
}
local function providerRows(starters)
  local result = {}
  if not starters then return result end
  for index, ids in pairs(starters) do
    local provider = providers[index]
    if provider then
      for _, id in ipairs(ids) do
        local entry = { type = provider[2], id = id }
        if provider[1].Exists(id) then
          entry.name = provider[1].Get(id, "name")
          local spawns = provider[1].Get(id, "spawns")
          if spawns then entry.spawns = spawns end
        end
        result[#result + 1] = entry
      end
    end
  end
  table.sort(result, function(a, b)
    if a.type == b.type then return a.id < b.id end
    return a.type < b.type
  end)
  return result
end

for line in assert(io.lines(idsPath)) do
  local id = tonumber(line)
  assert(id and id > 0 and id % 1 == 0, "invalid quest ID: " .. line)
  if db.Quest.Exists(id) then
    local fields, provenance = {}, {}
    for index = 1, db.Meta.Quest.fieldCount do
      local name = db.Meta.Quest.names[index]
      local value = db.Quest.Get(id, name)
      if value ~= nil then
        fields[name] = value
        provenance[name] = db.GetProvenance("Quest", id, name)
      end
    end
    local row = { fields = fields, provenance = provenance,
                  starters = providerRows(fields.startedBy),
                  finishers = providerRows(fields.finishedBy) }
    io.write(encode({ id = id, row = row }), "\n")
  else
    io.write(encode({ id = id, missing = true }), "\n")
  end
end
