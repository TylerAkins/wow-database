-- Export the composed Forever quest view as JSON lines.
-- Run with the working directory set to a QuestieDB checkout:
--   luajit /path/to/export_questiedb.lua <commit> <generatedAt>
local commit = assert(arg[1], "QuestieDB commit required")
local generatedAt = assert(arg[2], "generatedAt required")

local client = dofile("emulator/client.lua")
local emulator = dofile("emulator/metadata.lua")
client.install({ expansion = "Forever", locale = "enUS" })
local db = emulator.loadAddon("QuestieDB.toc", "QuestieDB")
assert(db.flavor.name == "Forever", "QuestieDB selected the wrong flavor")

local objectiveGroups = {
  { "creature", "npc" },
  { "object", "object" },
  { "item", "item" },
  { "reputation", nil },
  { "killCredit", "npc" },
  { "spell", nil },
}
local providerKinds = { "npc", "object", "item" }
local providerDB = { npc = db.Npc, object = db.Object, item = db.Item }

local function quote(value)
  return '"' .. value:gsub('[%z\1-\31\\"]', function(char)
    local escapes = {
      ["\\"] = "\\\\", ['"'] = '\\"', ["\n"] = "\\n",
      ["\r"] = "\\r", ["\t"] = "\\t",
    }
    return escapes[char] or string.format("\\u%04x", string.byte(char))
  end) .. '"'
end

local function isArray(value)
  local count, maximum = 0, 0
  for key in pairs(value) do
    if type(key) ~= "number" or key < 1 or key % 1 ~= 0 then return false end
    count = count + 1
    if key > maximum then maximum = key end
  end
  return count == maximum
end

local function encode(value)
  local kind = type(value)
  if kind == "nil" then return "null" end
  if kind == "string" then return quote(value) end
  if kind == "number" then
    if value ~= value or value == math.huge or value == -math.huge then
      error("cannot encode non-finite number")
    end
    if value % 1 == 0 then return string.format("%.0f", value) end
    return string.format("%.17g", value)
  end
  if kind == "boolean" then return value and "true" or "false" end
  assert(kind == "table", "unsupported JSON value: " .. kind)
  if isArray(value) then
    local parts = {}
    for index = 1, #value do parts[index] = encode(value[index]) end
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

local function flattenSpawns(spawns)
  local points = {}
  if type(spawns) ~= "table" then return points end
  for zoneId, coords in pairs(spawns) do
    local zone = tonumber(zoneId)
    if zone ~= nil and type(coords) == "table" then
      for _, pair in ipairs(coords) do
        if type(pair) == "table" and type(pair[1]) == "number" and type(pair[2]) == "number" then
          points[#points + 1] = { zone, pair[1], pair[2] }
        end
      end
    end
  end
  table.sort(points, function(a, b)
    if a[1] ~= b[1] then return a[1] < b[1] end
    if a[2] ~= b[2] then return a[2] < b[2] end
    return a[3] < b[3]
  end)
  return points
end

local function readField(entity, id, name)
  local ok, value = pcall(entity.Get, id, name)
  if not ok then error("Get failed for " .. name .. " " .. id .. ": " .. tostring(value)) end
  return value
end

local function entityName(kind, id)
  local entity = providerDB[kind]
  if not entity or not entity.Exists(id) then return nil end
  return readField(entity, id, "name")
end

local function entitySpawns(kind, id)
  local entity = providerDB[kind]
  if kind == "item" or not entity or not entity.Exists(id) then return {} end
  return flattenSpawns(readField(entity, id, "spawns"))
end

local function provider(kind, id)
  local row = { type = kind, id = id }
  local entity = providerDB[kind]
  if entity and entity.Exists(id) then
    row.name = entityName(kind, id)
    if kind ~= "item" then row.spawns = entitySpawns(kind, id) end
  end
  return row
end

local function addPlace(places, seen, place)
  local key = table.concat({
    place.role, place.type or "", tostring(place.id or ""),
    tostring(place.itemId or ""), place.name or "",
  }, "\0")
  if seen[key] then return end
  seen[key] = true
  if not place.spawns then place.spawns = {} end
  places[#places + 1] = place
end

local function itemSources(itemId)
  local sources = {}
  if not db.Item.Exists(itemId) then return sources end
  local function append(field, kind)
    local ids = readField(db.Item, itemId, field)
    if type(ids) ~= "table" then return end
    for _, id in ipairs(ids) do
      sources[#sources + 1] = { kind = kind, id = id }
    end
  end
  append("npcDrops", "npc")
  append("objectDrops", "object")
  append("vendors", "npc")
  return sources
end

local function addEntityPlace(places, seen, role, kind, id, itemId)
  local row = provider(kind, id)
  local place = {
    role = role, type = row.type, id = row.id, name = row.name, spawns = row.spawns or {},
  }
  if itemId then place.itemId = itemId end
  addPlace(places, seen, place)
end

local function addItemPlaces(places, seen, role, itemId)
  local sources = itemSources(itemId)
  if #sources == 0 then
    addEntityPlace(places, seen, role, "item", itemId, itemId)
    return
  end
  for _, source in ipairs(sources) do
    addEntityPlace(places, seen, role, source.kind, source.id, itemId)
  end
end

local function providerList(value, withItems)
  local rows = {}
  if type(value) ~= "table" then return rows end
  for index, ids in pairs(value) do
    local kind = providerKinds[index]
    if kind and (withItems or kind ~= "item") and type(ids) == "table" then
      for _, id in ipairs(ids) do
        rows[#rows + 1] = provider(kind, id)
      end
    end
  end
  table.sort(rows, function(a, b)
    if a.type ~= b.type then return a.type < b.type end
    return a.id < b.id
  end)
  return rows
end

local function namedObjective(group, row)
  if group == "reputation" then
    return { factionId = row[1], value = row[2] }
  end
  if group == "killCredit" then
    local npcIds = {}
    if type(row[1]) == "table" then
      for _, id in ipairs(row[1]) do npcIds[#npcIds + 1] = id end
    end
    local named = { npcIds = npcIds, creditId = row[2], text = row[3] }
    if row[4] and row[4] ~= 0 then named.count = row[4] end
    return named
  end
  if group == "spell" then
    local named = { spellId = row[1], text = row[2] }
    if row[3] and row[3] ~= 0 then named.itemId = row[3] end
    return named
  end
  local named = { id = row[1], text = row[2] }
  if row[3] and row[3] ~= 0 then named.count = row[3] end
  return named
end

local function nameObjectives(value)
  local named = {}
  if type(value) ~= "table" then return named end
  for index, group in ipairs(objectiveGroups) do
    local rows = value[index]
    if type(rows) == "table" then
      if group[1] == "reputation" and type(rows[1]) == "number" then
        named.reputation = namedObjective("reputation", rows)
      else
        local list = {}
        for _, row in ipairs(rows) do
          if type(row) == "table" then
            list[#list + 1] = namedObjective(group[1], row)
          end
        end
        if #list > 0 then named[group[1]] = list end
      end
    end
  end
  return named
end

local function objectivePlaces(places, seen, objectives)
  if type(objectives) ~= "table" then return end
  local creature = objectives[1]
  if type(creature) == "table" then
    for _, row in ipairs(creature) do
      if type(row) == "table" and row[1] then
        addEntityPlace(places, seen, "objective", "npc", row[1])
      end
    end
  end
  local objects = objectives[2]
  if type(objects) == "table" then
    for _, row in ipairs(objects) do
      if type(row) == "table" and row[1] then
        addEntityPlace(places, seen, "objective", "object", row[1])
      end
    end
  end
  local items = objectives[3]
  if type(items) == "table" then
    for _, row in ipairs(items) do
      if type(row) == "table" and row[1] then addItemPlaces(places, seen, "objective", row[1]) end
    end
  end
  local credits = objectives[5]
  if type(credits) == "table" then
    for _, row in ipairs(credits) do
      if type(row) == "table" then
        if type(row[1]) == "table" then
          for _, id in ipairs(row[1]) do addEntityPlace(places, seen, "objective", "npc", id) end
        end
        if row[2] then addEntityPlace(places, seen, "objective", "npc", row[2]) end
      end
    end
  end
end

local function coordinatePlace(role, name, spawns)
  return { role = role, type = "coordinate", id = 0, name = name, spawns = spawns }
end

local function extraPlaces(places, seen, rows)
  if type(rows) ~= "table" then return end
  for _, row in ipairs(rows) do
    if type(row) == "table" then
      local spawns = flattenSpawns(row[1])
      if #spawns > 0 then
        addPlace(places, seen, coordinatePlace("extra", row[3], spawns))
      end
      local refs = row[5]
      if type(refs) == "table" and #spawns == 0 then
        for _, ref in ipairs(refs) do
          if type(ref) == "table" and ref[1] and ref[2] then
            local kind = ref[1] == "monster" and "npc" or ref[1]
            if providerDB[kind] then addEntityPlace(places, seen, "extra", kind, ref[2]) end
          end
        end
      end
    end
  end
end

local function nameExtra(rows)
  local named = {}
  if type(rows) ~= "table" then return named end
  for _, row in ipairs(rows) do
    if type(row) == "table" then
      local refs = {}
      if type(row[5]) == "table" then
        for _, ref in ipairs(row[5]) do
          if type(ref) == "table" then
            refs[#refs + 1] = { type = ref[1], id = ref[2] }
          end
        end
      end
      named[#named + 1] = {
        text = row[3], icon = row[2], objectiveIndex = row[4],
        spawns = flattenSpawns(row[1]), references = refs,
      }
    end
  end
  return named
end

local function nameTrigger(value)
  if type(value) ~= "table" then return nil end
  return { text = value[1], spawns = flattenSpawns(value[2]) }
end

local function copyField(target, name, value)
  if value == nil or value == 0 or value == "" then return end
  if type(value) == "table" and next(value) == nil then return end
  target[name] = value
end

local ids = db.Quest.GetAllIds()
local questCount = #ids
if questCount == 0 then error("Quest.GetAllIds returned no Forever quests") end
table.sort(ids)

local written = 0
for _, id in ipairs(ids) do
  if not db.Quest.Exists(id) then error("GetAllIds returned missing quest " .. id) end
  local fields = {}
  for index = 1, db.Meta.Quest.fieldCount do
    local name = db.Meta.Quest.names[index]
    copyField(fields, name, readField(db.Quest, id, name))
  end
  local startedBy = providerList(fields.startedBy, true)
  local finishedBy = providerList(fields.finishedBy, false)
  local places, seen = {}, {}
  for _, row in ipairs(startedBy) do
    if row.type == "item" then
      addItemPlaces(places, seen, "available", row.id)
    else
      addPlace(places, seen, {
        role = "available", type = row.type, id = row.id, name = row.name, spawns = row.spawns or {},
      })
    end
  end
  for _, row in ipairs(finishedBy) do
    addPlace(places, seen, {
      role = "turnIn", type = row.type, id = row.id, name = row.name, spawns = row.spawns or {},
    })
  end
  objectivePlaces(places, seen, fields.objectives)
  local trigger = nameTrigger(fields.triggerEnd)
  if trigger and #trigger.spawns > 0 then
    addPlace(places, seen, coordinatePlace("trigger", trigger.text, trigger.spawns))
  end
  extraPlaces(places, seen, fields.extraObjectives)
  fields.objectives = nameObjectives(fields.objectives)
  fields.triggerEnd = trigger
  fields.extraObjectives = nameExtra(fields.extraObjectives)
  io.write(encode({
    id = id, fields = fields, startedBy = startedBy, finishedBy = finishedBy, places = places,
  }), "\n")
  written = written + 1
  if written % 1000 == 0 then io.stderr:write("exported " .. written .. "\n") end
end

if written ~= questCount then
  error("exported " .. written .. " quests; GetAllIds count is " .. questCount)
end
io.write(encode({
  kind = "summary", questCount = questCount, commit = commit, generatedAt = generatedAt,
}), "\n")
io.stderr:write("questCount=" .. questCount .. "\n")
