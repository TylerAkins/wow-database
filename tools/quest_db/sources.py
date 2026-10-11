"""Canonical Source Forever quest index URLs and taxonomy."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Literal

SourceKind = Literal[
    "objects",
    "zone",
    "class",
    "dungeon",
    "raid",
    "world_event",
    "profession",
    "uncategorized",
    "battleground",
]


@dataclass(frozen=True, slots=True)
class SourcePage:
    url: str
    kind: SourceKind
    slug: str

    @property
    def path(self) -> str:
        return self.url.split("/forever/", 1)[-1].rstrip("/")


# Keep in sync with docs/quest-database.md
SOURCE_PAGES: tuple[SourcePage, ...] = (
    SourcePage("https://www.wowhead.com/forever/objects/quests", "objects", "objects/quests"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/alterac-mountains", "zone", "ek/alterac-mountains"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/alterac-valley", "zone", "ek/alterac-valley"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/anvilmar", "zone", "ek/anvilmar"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/arathi-highlands", "zone", "ek/arathi-highlands"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/badlands", "zone", "ek/badlands"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/blackrock-mountain", "zone", "ek/blackrock-mountain"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/blasted-lands", "zone", "ek/blasted-lands"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/burning-steppes", "zone", "ek/burning-steppes"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/crafting", "zone", "ek/crafting"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/deeprun-tram", "zone", "ek/deeprun-tram"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/dun-morogh", "zone", "ek/dun-morogh"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/duskwood", "zone", "ek/duskwood"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/eastern-plaguelands", "zone", "ek/eastern-plaguelands"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/elwynn-forest", "zone", "ek/elwynn-forest"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/hillsbrad-foothills", "zone", "ek/hillsbrad-foothills"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/ironforge", "zone", "ek/ironforge"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/kharanos", "zone", "ek/kharanos"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/loch-modan", "zone", "ek/loch-modan"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/redridge-mountains", "zone", "ek/redridge-mountains"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/riverglades", "zone", "ek/riverglades"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/searing-gorge", "zone", "ek/searing-gorge"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/shadowfang-keep", "zone", "ek/shadowfang-keep"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/silverpine-forest", "zone", "ek/silverpine-forest"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/stormwind-city", "zone", "ek/stormwind-city"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/stonewrought-dam", "zone", "ek/stonewrought-dam"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/stranglethorn-vale", "zone", "ek/stranglethorn-vale"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/swamp-of-sorrows", "zone", "ek/swamp-of-sorrows"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/the-hinterlands", "zone", "ek/the-hinterlands"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/thoradins-wall", "zone", "ek/thoradins-wall"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/tirisfal-glades", "zone", "ek/tirisfal-glades"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/undercity", "zone", "ek/undercity"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/western-plaguelands", "zone", "ek/western-plaguelands"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/westfall", "zone", "ek/westfall"),
    SourcePage("https://www.wowhead.com/forever/quests/eastern-kingdoms/wetlands", "zone", "ek/wetlands"),
    SourcePage("https://www.wowhead.com/forever/quests/kalimdor/abyssal-sands", "zone", "kd/abyssal-sands"),
    SourcePage("https://www.wowhead.com/forever/quests/kalimdor/ashenvale", "zone", "kd/ashenvale"),
    SourcePage("https://www.wowhead.com/forever/quests/kalimdor/azshara", "zone", "kd/azshara"),
    SourcePage("https://www.wowhead.com/forever/quests/kalimdor/blackmaw-hold", "zone", "kd/blackmaw-hold"),
    SourcePage("https://www.wowhead.com/forever/quests/kalimdor/darkshore", "zone", "kd/darkshore"),
    SourcePage("https://www.wowhead.com/forever/quests/kalimdor/darnassus", "zone", "kd/darnassus"),
    SourcePage("https://www.wowhead.com/forever/quests/kalimdor/desolace", "zone", "kd/desolace"),
    SourcePage("https://www.wowhead.com/forever/quests/kalimdor/durotar", "zone", "kd/durotar"),
    SourcePage("https://www.wowhead.com/forever/quests/kalimdor/dustwallow-marsh", "zone", "kd/dustwallow-marsh"),
    SourcePage("https://www.wowhead.com/forever/quests/kalimdor/felwood", "zone", "kd/felwood"),
    SourcePage("https://www.wowhead.com/forever/quests/kalimdor/feralas", "zone", "kd/feralas"),
    SourcePage("https://www.wowhead.com/forever/quests/kalimdor/field-of-giants", "zone", "kd/field-of-giants"),
    SourcePage("https://www.wowhead.com/forever/quests/kalimdor/moonglade", "zone", "kd/moonglade"),
    SourcePage("https://www.wowhead.com/forever/quests/kalimdor/mulgore", "zone", "kd/mulgore"),
    SourcePage("https://www.wowhead.com/forever/quests/kalimdor/orgrimmar", "zone", "kd/orgrimmar"),
    SourcePage("https://www.wowhead.com/forever/quests/kalimdor/ruttheran-village", "zone", "kd/ruttheran-village"),
    SourcePage("https://www.wowhead.com/forever/quests/kalimdor/silithus", "zone", "kd/silithus"),
    SourcePage("https://www.wowhead.com/forever/quests/kalimdor/stonetalon-mountains", "zone", "kd/stonetalon-mountains"),
    SourcePage("https://www.wowhead.com/forever/quests/kalimdor/tanaris", "zone", "kd/tanaris"),
    SourcePage("https://www.wowhead.com/forever/quests/kalimdor/teldrassil", "zone", "kd/teldrassil"),
    SourcePage("https://www.wowhead.com/forever/quests/kalimdor/the-barrens", "zone", "kd/the-barrens"),
    SourcePage("https://www.wowhead.com/forever/quests/kalimdor/thousand-needles", "zone", "kd/thousand-needles"),
    SourcePage("https://www.wowhead.com/forever/quests/kalimdor/thunder-bluff", "zone", "kd/thunder-bluff"),
    SourcePage("https://www.wowhead.com/forever/quests/kalimdor/ungoro-crater", "zone", "kd/ungoro-crater"),
    SourcePage("https://www.wowhead.com/forever/quests/kalimdor/winterspring", "zone", "kd/winterspring"),
    SourcePage("https://www.wowhead.com/forever/quests/classes/druid", "class", "class/druid"),
    SourcePage("https://www.wowhead.com/forever/quests/classes/hunter", "class", "class/hunter"),
    SourcePage("https://www.wowhead.com/forever/quests/classes/mage", "class", "class/mage"),
    SourcePage("https://www.wowhead.com/forever/quests/classes/paladin", "class", "class/paladin"),
    SourcePage("https://www.wowhead.com/forever/quests/classes/priest", "class", "class/priest"),
    SourcePage("https://www.wowhead.com/forever/quests/classes/rogue", "class", "class/rogue"),
    SourcePage("https://www.wowhead.com/forever/quests/classes/shaman", "class", "class/shaman"),
    SourcePage("https://www.wowhead.com/forever/quests/classes/warlock", "class", "class/warlock"),
    SourcePage("https://www.wowhead.com/forever/quests/classes/warrior", "class", "class/warrior"),
    SourcePage("https://www.wowhead.com/forever/quests/dungeons/blackfathom-deeps", "dungeon", "dungeon/blackfathom-deeps"),
    SourcePage("https://www.wowhead.com/forever/quests/dungeons/blackrock-depths", "dungeon", "dungeon/blackrock-depths"),
    SourcePage("https://www.wowhead.com/forever/quests/dungeons/blackrock-spire", "dungeon", "dungeon/blackrock-spire"),
    SourcePage("https://www.wowhead.com/forever/quests/dungeons/dire-maul", "dungeon", "dungeon/dire-maul"),
    SourcePage("https://www.wowhead.com/forever/quests/dungeons/gnomeregan", "dungeon", "dungeon/gnomeregan"),
    SourcePage("https://www.wowhead.com/forever/quests/dungeons/maraudon", "dungeon", "dungeon/maraudon"),
    SourcePage("https://www.wowhead.com/forever/quests/dungeons/ragefire-chasm", "dungeon", "dungeon/ragefire-chasm"),
    SourcePage("https://www.wowhead.com/forever/quests/dungeons/razorfen-downs", "dungeon", "dungeon/razorfen-downs"),
    SourcePage("https://www.wowhead.com/forever/quests/dungeons/razorfen-kraul", "dungeon", "dungeon/razorfen-kraul"),
    SourcePage("https://www.wowhead.com/forever/quests/dungeons/ruins-of-lordaeron", "dungeon", "dungeon/ruins-of-lordaeron"),
    SourcePage("https://www.wowhead.com/forever/quests/dungeons/scarlet-monastery", "dungeon", "dungeon/scarlet-monastery"),
    SourcePage("https://www.wowhead.com/forever/quests/dungeons/scholomance", "dungeon", "dungeon/scholomance"),
    SourcePage("https://www.wowhead.com/forever/quests/dungeons/shadowfang-keep", "dungeon", "dungeon/shadowfang-keep"),
    SourcePage("https://www.wowhead.com/forever/quests/dungeons/stratholme", "dungeon", "dungeon/stratholme"),
    SourcePage("https://www.wowhead.com/forever/quests/dungeons/the-deadmines", "dungeon", "dungeon/the-deadmines"),
    SourcePage("https://www.wowhead.com/forever/quests/dungeons/the-hall-of-thanes", "dungeon", "dungeon/the-hall-of-thanes"),
    SourcePage("https://www.wowhead.com/forever/quests/dungeons/the-stockade", "dungeon", "dungeon/the-stockade"),
    SourcePage("https://www.wowhead.com/forever/quests/dungeons/the-temple-of-atalhakkar", "dungeon", "dungeon/the-temple-of-atalhakkar"),
    SourcePage("https://www.wowhead.com/forever/quests/dungeons/uldaman", "dungeon", "dungeon/uldaman"),
    SourcePage("https://www.wowhead.com/forever/quests/dungeons/wailing-caverns", "dungeon", "dungeon/wailing-caverns"),
    SourcePage("https://www.wowhead.com/forever/quests/dungeons/zulfarrak", "dungeon", "dungeon/zulfarrak"),
    SourcePage("https://www.wowhead.com/forever/quests/raids/onyxias-lair", "raid", "raid/onyxias-lair"),
    SourcePage("https://www.wowhead.com/forever/quests/world-events/childrens-week", "world_event", "event/childrens-week"),
    SourcePage("https://www.wowhead.com/forever/quests/world-events/darkmoon-faire", "world_event", "event/darkmoon-faire"),
    SourcePage("https://www.wowhead.com/forever/quests/world-events/hallows-end", "world_event", "event/hallows-end"),
    SourcePage("https://www.wowhead.com/forever/quests/world-events/love-is-in-the-air", "world_event", "event/love-is-in-the-air"),
    SourcePage("https://www.wowhead.com/forever/quests/world-events/lunar-festival", "world_event", "event/lunar-festival"),
    SourcePage("https://www.wowhead.com/forever/quests/world-events/midsummer", "world_event", "event/midsummer"),
    SourcePage("https://www.wowhead.com/forever/quests/world-events/winter-veil", "world_event", "event/winter-veil"),
    SourcePage("https://www.wowhead.com/forever/quests/professions/alchemy", "profession", "profession/alchemy"),
    SourcePage("https://www.wowhead.com/forever/quests/professions/blacksmithing", "profession", "profession/blacksmithing"),
    SourcePage("https://www.wowhead.com/forever/quests/professions/cooking", "profession", "profession/cooking"),
    SourcePage("https://www.wowhead.com/forever/quests/professions/engineering", "profession", "profession/engineering"),
    SourcePage("https://www.wowhead.com/forever/quests/professions/first-aid", "profession", "profession/first-aid"),
    SourcePage("https://www.wowhead.com/forever/quests/professions/fishing", "profession", "profession/fishing"),
    SourcePage("https://www.wowhead.com/forever/quests/professions/herbalism", "profession", "profession/herbalism"),
    SourcePage("https://www.wowhead.com/forever/quests/professions/leatherworking", "profession", "profession/leatherworking"),
    SourcePage("https://www.wowhead.com/forever/quests/professions/tailoring", "profession", "profession/tailoring"),
    SourcePage("https://www.wowhead.com/forever/quests/uncategorized", "uncategorized", "uncategorized"),
    SourcePage("https://www.wowhead.com/forever/quests/battlegrounds/alterac-valley", "battleground", "bg/alterac-valley"),
    SourcePage("https://www.wowhead.com/forever/quests/battlegrounds/arathi-basin", "battleground", "bg/arathi-basin"),
    SourcePage("https://www.wowhead.com/forever/quests/battlegrounds/warsong-gulch", "battleground", "bg/warsong-gulch"),
)


def source_by_slug(slug: str) -> SourcePage | None:
    for page in SOURCE_PAGES:
        if page.slug == slug:
            return page
    return None
