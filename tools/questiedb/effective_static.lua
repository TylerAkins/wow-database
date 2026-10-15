-- Run from the upstream checkout. Only Static Corrections are applied.
assert(_VERSION == "Lua 5.1", "QuestieDB import requires Lua 5.1")
local output = assert(io.open(arg[1], "wb"))
local config = dofile("src/config.lua")
local runtime = dofile("generator/runtime.lua")
local serialize = dofile("generator/serialize.lua")
local flavor = assert(config.flavorByName.Forever)
local loaded = dofile("generator/flavor.lua").load(flavor, nil, false)
local db = runtime.build()
runtime.loadCorrections(db, flavor)
for _, spec in ipairs(db.CorrectionManifest) do
  if config.correctionApplies(spec, flavor) then
    assert(spec.loaded, "Registered correction module was not loaded: " .. spec.file)
    local module = db.CorrectionCompat.modules[spec.module]
    for _, method in ipairs(spec.static or {}) do
      assert(type(module[method]) == "function", "Missing static provider: " .. spec.file .. ":" .. method)
    end
  end
end
local providers = {}
local provenance = {}
local originalMerge = db.Corrections.MergeInto
local originalResolve = db.TablePatch.Resolve
-- Observe the operation engine before MergeInto writes. This also records same-value writes.
db.Corrections.MergeInto = function(entities, corrections, options, context)
  options = options or {}
  local writes = {}
  db.TablePatch.Resolve = function(fields, meta, ctx, id, base, fallback, noOverwrites, sentinel)
    local resolved = originalResolve(fields, meta, ctx, id, base, fallback, noOverwrites, sentinel)
    local selected = {}
    for field in pairs(resolved) do
      if type(field) == "number" and (not options.noOverwrites or not base or base[field] == nil) then
        selected[field] = true
      end
    end
    writes[id] = selected
    return resolved
  end
  local applied = originalMerge(entities, corrections, options, context)
  db.TablePatch.Resolve = originalResolve
  local byType = provenance[context.datatype] or {}
  provenance[context.datatype] = byType
  for id, fields in pairs(writes) do
    if entities[id] then
      local row = byType[id] or {}
      byType[id] = row
      for field in pairs(fields) do row[field] = context.name end
    end
  end
  return applied
end
for _, entry in ipairs(db.Corrections.Select({dynamic = false, owner = db.Corrections.OWNER})) do
  if db.Corrections.EntryApplies(entry, flavor) then
    providers[#providers + 1] = {name = entry.name, datatype = entry.datatype, loadOrder = entry.loadOrder}
  end
end
local structures = {}
for kind, entry in pairs(loaded) do
  local fields = {}
  for field, shape in pairs(entry.meta.structures) do fields[entry.meta.names[field]] = shape end
  structures[kind:lower()] = fields
end
output:write(serialize.value({staticProviders = providers, fieldStructures = structures}), "\n")
for _, entityType in ipairs(config.entityTypes) do
  local kind = entityType.name
  local entry = loaded[kind]
  db.Corrections.ApplyStaticToEntities(kind, entry.entities, flavor, db.Corrections.OWNER)
  local ids = {}
  for id in pairs(entry.entities) do ids[#ids + 1] = id end
  table.sort(ids)
  for _, id in ipairs(ids) do
    local attributes, states, paths, scalarClears = {}, {}, {}, {}
    local rowProvenance = provenance[kind] and provenance[kind][id] or {}
    for field = 1, entry.meta.fieldCount do
      local name = entry.meta.names[field]
      local value = entry.entities[id][field]
      if value == nil then
        states[name] = "missing"
      elseif type(value) == "table" and next(value) == nil then
        states[name] = "empty"
        if entry.meta.types[field] == "table" then attributes[name] = value
        else scalarClears[#scalarClears + 1] = name end
      else
        attributes[name] = value
      end
      local provider = rowProvenance[field]
      paths[name] = provider and {path = "src/corrections/" .. provider:match("^(.*):[^:]+$"), provider = provider}
        or {path = entry.path}
    end
    output:write(serialize.value({entityType = kind:lower(), entityId = tostring(id), attributes = attributes,
      fieldStates = states, provenance = paths, scalarClears = scalarClears}), "\n")
  end
end
assert(output:close())
