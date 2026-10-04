package compile

// Quest flag bits from QuestieDB src/corrections/enum/quests.lua.
const (
	questFlagStayAlive     = 1
	questFlagPartyAccept   = 2
	questFlagExploration   = 4
	questFlagSharable      = 8
	questFlagUnused1       = 16
	questFlagEpic          = 32
	questFlagRaid          = 64
	questFlagUnused2       = 128
	questFlagUnknown       = 256
	questFlagHiddenRewards = 512
	questFlagAutoRewarded  = 1024
	questFlagDaily         = 4096
	questFlagWeekly        = 32768
	questFlagMonthly       = 65536
)

// specialFlags from QuestieDB. Repeatable is constants.specialFlags.REPEATABLE.
// Event is the event-gated bit named in src/meta/questMeta.lua.
const (
	questSpecialRepeatable = 1
	questSpecialEvent      = 2
)

const knownQuestFlags = questFlagStayAlive | questFlagPartyAccept | questFlagExploration |
	questFlagSharable | questFlagUnused1 | questFlagEpic | questFlagRaid | questFlagUnused2 |
	questFlagUnknown | questFlagHiddenRewards | questFlagAutoRewarded | questFlagDaily |
	questFlagWeekly | questFlagMonthly

// Battleground area ids that also appear in support/Forever/Zones/dungeons.lua.
const (
	areaAlteracValley = 2597
	areaWarsongGulch  = 3277
	areaArathiBasin   = 3358
)

type bitID struct {
	bit uint64
	id  int
}

var raceBits = []bitID{
	{1, 1}, {2, 2}, {4, 3}, {8, 4}, {16, 5}, {32, 6}, {64, 7}, {128, 8},
	{256, 9}, {4294967296, 95}, {8589934592, 96},
}

var classBits = []bitID{
	{1, 1}, {2, 2}, {4, 3}, {8, 4}, {16, 5}, {64, 7}, {128, 8}, {256, 9}, {1024, 11},
}

var allianceRaces = map[int]bool{1: true, 3: true, 4: true, 7: true, 95: true}
var hordeRaces = map[int]bool{2: true, 5: true, 6: true, 8: true, 9: true, 96: true}

// sortKeys from QuestieDB src/corrections/enum/quests.lua. Negative zoneOrSort values.
var sortKeys = map[int]string{
	-1000: "SPECIALTEMP", -367: "REPUTATION", -344: "LEGENDARY", -284: "SPECIAL",
	-221: "TREASURE_MAP", -23: "UNDERCITY", -1: "EPIC",
	-395: "MONK", -372: "DEATHKNIGHT", -263: "DRUID", -262: "PRIEST", -261: "HUNTER",
	-162: "ROGUE", -161: "MAGE", -141: "PALADIN", -82: "SHAMAN", -81: "WARRIOR", -61: "WARLOCK",
	-398: "RIDING", -377: "ARCHAEOLOGY", -373: "JEWELCRAFTING", -371: "INSCRIPTION",
	-324: "FIRST_AID", -304: "COOKING", -264: "TAILORING", -201: "ENGINEERING",
	-182: "LEATHERWORKING", -181: "ALCHEMY", -121: "BLACKSMITHING", -101: "FISHING", -24: "HERBALISM",
	-404: "WINTER_VEIL", -402: "HARVEST_FESTIVAL", -378: "CHILDRENS_WEEK",
	-376: "LOVE_IS_IN_THE_AIR", -375: "PILGRIMS_BOUNTY", -374: "NOBLEGARDEN", -370: "BREWFEST",
	-369: "MIDSUMMER", -366: "LUNAR_FESTIVAL", -364: "DARKMOON_FAIRE", -41: "DAY_OF_THE_DEAD",
	-22: "SEASONAL", -21: "HALLOWS_END",
	-368: "INVASION", -365: "AHN_QIRAJ_WAR", -241: "TOURNAMENT", -25: "BATTLEGROUNDS",
	-381: "ELEMENTAL_BONDS", -380: "THE_ZANDALARI", -379: "FIRELANDS_INVASION",
	-400: "PROVING_GROUNDS", -399: "BRAWLERS_GUILD", -397: "PANDAREN_CAMPAIGN", -396: "LANDFALL",
	-394: "PET_BATTLE", -392: "SCENARIO", -391: "PANDAREN_BREWMASTERS",
	-662: "TITAN_REFORGED_REALM", -644: "BLACKROCK_ERUPTION", -641: "NIGHTMARE_INCURSIONS",
	-676: "NIGHT_ELF", -666: "CAMPING", -660: "THE_HIGH_ORDER",
}

// professionKeys from QuestieDB src/corrections/enum/professions.lua.
var professionKeys = map[int]string{
	164: "BLACKSMITHING", 165: "LEATHERWORKING", 171: "ALCHEMY", 197: "TAILORING",
	202: "ENGINEERING", 333: "ENCHANTING", 755: "JEWELCRAFTING", 773: "INSCRIPTION",
	182: "HERBALISM", 186: "MINING", 393: "SKINNING",
	129: "FIRST_AID", 185: "COOKING", 356: "FISHING", 794: "ARCHAEOLOGY", 762: "RIDING",
}

// dungeonAreas is the Forever dungeon and raid area set from
// support/Forever/Zones/dungeons.lua, including alternativeAreaIds.
// Alterac Valley, Warsong Gulch, and Arathi Basin are omitted.
var dungeonAreas = map[int]bool{
	133: true, 206: true, 209: true, 491: true, 717: true, 718: true, 719: true, 721: true, 722: true, 796: true, 978: true, 1176: true,
	1196: true, 1337: true, 1417: true, 1477: true, 1517: true, 1581: true, 1583: true, 1584: true, 1585: true, 1717: true, 1977: true, 2017: true,
	2057: true, 2100: true, 2159: true, 2257: true, 2366: true, 2367: true, 2437: true, 2557: true, 2677: true, 2717: true, 2917: true, 2918: true,
	3428: true, 3429: true, 3456: true, 3457: true, 3562: true, 3606: true, 3607: true, 3713: true, 3714: true, 3715: true, 3716: true, 3717: true,
	3789: true, 3790: true, 3791: true, 3792: true, 3805: true, 3836: true, 3845: true, 3847: true, 3848: true, 3849: true, 3923: true, 3959: true,
	4075: true, 4100: true, 4131: true, 4196: true, 4228: true, 4264: true, 4265: true, 4272: true, 4273: true, 4277: true, 4415: true, 4416: true,
	4493: true, 4494: true, 4500: true, 4603: true, 4657: true, 4722: true, 4723: true, 4809: true, 4812: true, 4813: true, 4820: true, 4926: true,
	4945: true, 4950: true, 4987: true, 5004: true, 5035: true, 5088: true, 5094: true, 5334: true, 5396: true, 5600: true, 5638: true, 5723: true,
	5733: true, 5788: true, 5789: true, 5790: true, 5792: true, 5793: true, 5794: true, 5795: true, 5844: true, 5861: true, 5892: true, 5918: true,
	5956: true, 5963: true, 5976: true, 6052: true, 6066: true, 6067: true, 6109: true, 6125: true, 6173: true, 6182: true, 6214: true, 6297: true,
	6298: true, 6386: true, 6618: true, 6622: true, 6738: true, 10000: true, 10001: true, 10003: true, 10004: true, 10005: true, 10006: true, 10007: true,
	10008: true, 10009: true, 10010: true, 10011: true, 10012: true, 10013: true, 10014: true, 10015: true, 10016: true, 10017: true, 10018: true, 10019: true,
	10020: true, 10021: true, 10022: true, 10023: true, 10024: true, 10025: true, 10026: true, 10027: true, 10028: true, 10029: true, 10030: true, 10031: true,
	10032: true, 10033: true, 10039: true, 10040: true, 10041: true, 10042: true, 10043: true, 10044: true, 10045: true, 10046: true, 10047: true, 10048: true,
	10049: true, 10050: true, 10051: true, 10052: true, 10053: true, 10054: true, 10055: true, 10056: true, 10057: true, 10058: true, 10059: true, 10060: true,
	10061: true, 10062: true, 10063: true, 10064: true, 10065: true, 10066: true, 10067: true, 10068: true, 10069: true, 10070: true, 10071: true, 10072: true,
	10076: true, 10077: true, 10078: true, 10079: true, 10093: true, 10094: true, 10095: true, 10096: true, 10097: true, 10098: true, 10099: true, 10103: true,
	10104: true, 10105: true, 10106: true, 10107: true, 10108: true, 10109: true, 10110: true, 10111: true, 10112: true, 10113: true, 10114: true, 10115: true,
	10116: true, 10117: true, 10118: true, 15475: true, 15531: true, 15828: true, 16074: true, 16236: true,
}
