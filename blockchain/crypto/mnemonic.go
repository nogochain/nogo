// Copyright 2026 The NogoChain Authors
// This file is part of the NogoChain library.
//
// The NogoChain library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The NogoChain library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the NogoChain library. If not, see <http://www.gnu.org/licenses/>.

package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/pbkdf2"
)

const (
	// MnemonicWordCount12 is for 12-word mnemonics (128 bits entropy)
	MnemonicWordCount12 = 12

	// MnemonicWordCount15 is for 15-word mnemonics (160 bits entropy)
	MnemonicWordCount15 = 15

	// MnemonicWordCount18 is for 18-word mnemonics (192 bits entropy)
	MnemonicWordCount18 = 18

	// MnemonicWordCount21 is for 21-word mnemonics (224 bits entropy)
	MnemonicWordCount21 = 21

	// MnemonicWordCount24 is for 24-word mnemonics (256 bits entropy)
	MnemonicWordCount24 = 24

	// MnemonicEntropyBits128 is 128 bits for 12 words
	MnemonicEntropyBits128 = 128

	// MnemonicEntropyBits256 is 256 bits for 24 words
	MnemonicEntropyBits256 = 256

	// MnemonicChecksumBits128 is checksum size for 128 bits (4 bits)
	MnemonicChecksumBits128 = MnemonicEntropyBits128 / 32

	// PBKDF2Iterations is the BIP39 standard iterations
	PBKDF2Iterations = 2048

	// MnemonicSaltPrefix is the BIP39 salt prefix
	MnemonicSaltPrefix = "mnemonic"
)

var (
	// ErrInvalidMnemonicWordCount is returned for invalid word count
	ErrInvalidMnemonicWordCount = errors.New("invalid mnemonic word count")

	// ErrInvalidMnemonicWord is returned for invalid word
	ErrInvalidMnemonicWord = errors.New("invalid mnemonic word")

	// ErrInvalidMnemonicChecksum is returned for checksum mismatch
	ErrInvalidMnemonicChecksum = errors.New("invalid mnemonic checksum")

	// ErrEntropyTooShort is returned when entropy is too short
	ErrEntropyTooShort = errors.New("entropy too short")

	// ErrInvalidMnemonicStrength is returned for invalid strength
	ErrInvalidMnemonicStrength = errors.New("invalid mnemonic strength")
)

// bip39Wordlist is the complete BIP39 English wordlist (2048 words)
var bip39Wordlist = []string{
	"abandon", "ability", "able", "about", "above", "absent", "absorb", "abstract", "absurd", "abuse",
	"access", "accident", "account", "accuse", "achieve", "acid", "acoustic", "acquire", "across", "act",
	"action", "actor", "actress", "actual", "adapt", "add", "addict", "address", "adjust", "admit",
	"adult", "advance", "advice", "aerobic", "affair", "afford", "afraid", "again", "age", "agent",
	"agree", "ahead", "aim", "air", "airport", "aisle", "alarm", "album", "alcohol", "alert",
	"alien", "all", "alley", "allow", "almost", "alone", "alpha", "already", "also", "alter",
	"always", "amateur", "amazing", "among", "amount", "amused", "analyst", "anchor", "ancient", "anger",
	"angle", "angry", "animal", "ankle", "announce", "annual", "another", "answer", "antenna", "antique",
	"anxiety", "any", "apart", "apology", "appear", "apple", "approve", "april", "arch", "arctic",
	"area", "arena", "argue", "arm", "armed", "armor", "army", "around", "arrange", "arrest",
	"arrive", "arrow", "art", "artefact", "artist", "artwork", "ask", "aspect", "assault", "asset",
	"assist", "assume", "asthma", "athlete", "atom", "attack", "attend", "attitude", "attract", "auction",
	"audit", "august", "aunt", "author", "auto", "autumn", "average", "avocado", "avoid", "awake",
	"aware", "away", "awesome", "awful", "awkward", "axis", "baby", "bachelor", "bacon", "badge",
	"bag", "balance", "balcony", "ball", "bamboo", "banana", "banner", "bar", "barely", "bargain",
	"barrel", "base", "basic", "basket", "battle", "beach", "bean", "beauty", "because", "become",
	"beef", "before", "begin", "behave", "behind", "believe", "below", "belt", "bench", "benefit",
	"best", "betray", "better", "between", "beyond", "bicycle", "bid", "bike", "bind", "biology",
	"bird", "birth", "bitter", "black", "blade", "blame", "blanket", "blast", "bleak", "bless",
	"blind", "blood", "blossom", "blouse", "blue", "blur", "blush", "board", "boat", "body",
	"boil", "bomb", "bone", "bonus", "book", "boost", "border", "boring", "borrow", "boss",
	"bottom", "bounce", "box", "boy", "bracket", "brain", "brand", "brass", "brave", "bread",
	"breeze", "brick", "bridge", "brief", "bright", "bring", "brisk", "broccoli", "broken", "bronze",
	"broom", "brother", "brown", "brush", "bubble", "buddy", "budget", "buffalo", "build", "bulb",
	"bulk", "bullet", "bundle", "bunker", "burden", "burger", "burst", "bus", "business", "busy",
	"butter", "buyer", "buzz", "cabbage", "cabin", "cable", "cactus", "cage", "cake", "call",
	"calm", "camera", "camp", "can", "canal", "cancel", "candy", "cannon", "canoe", "canvas",
	"canyon", "capable", "capital", "captain", "car", "carbon", "card", "cargo", "carpet", "carry",
	"cart", "case", "cash", "casino", "castle", "casual", "cat", "catalog", "catch", "category",
	"cattle", "caught", "cause", "caution", "cave", "ceiling", "celery", "cement", "census", "century",
	"cereal", "certain", "chair", "chalk", "champion", "change", "chaos", "chapter", "charge", "chase",
	"chat", "cheap", "check", "cheese", "chef", "cherry", "chest", "chicken", "chief", "child",
	"chimney", "choice", "choose", "chronic", "chuckle", "chunk", "churn", "cigar", "cinnamon", "circle",
	"citizen", "city", "civil", "claim", "clap", "clarify", "claw", "clay", "clean", "clerk",
	"clever", "click", "client", "cliff", "climb", "clinic", "clip", "clock", "clog", "close",
	"cloth", "cloud", "clown", "club", "clump", "cluster", "clutch", "coach", "coast", "coconut",
	"code", "coffee", "coil", "coin", "collect", "color", "column", "combine", "come", "comfort",
	"comic", "common", "company", "concert", "conduct", "confirm", "congress", "connect", "consider", "control",
	"convince", "cook", "cool", "copper", "copy", "coral", "core", "corn", "corner", "correct",
	"cost", "cotton", "couch", "country", "couple", "course", "cousin", "cover", "coyote", "crack",
	"cradle", "craft", "cram", "crane", "crash", "crater", "crawl", "crazy", "cream", "credit",
	"creek", "crew", "cricket", "crime", "crisp", "critic", "crop", "cross", "crouch", "crowd",
	"crucial", "cruel", "cruise", "crumble", "crunch", "crush", "cry", "crystal", "cube", "culture",
	"cup", "cupboard", "curious", "current", "curtain", "curve", "cushion", "custom", "cute", "cycle",
	"dad", "damage", "damp", "dance", "danger", "daring", "dash", "daughter", "dawn", "day",
	"deal", "debate", "debris", "decade", "december", "decide", "decline", "decorate", "decrease", "deer",
	"defense", "define", "defy", "degree", "delay", "deliver", "demand", "demise", "denial", "dentist",
	"deny", "depart", "depend", "deposit", "depth", "deputy", "derive", "describe", "desert", "design",
	"desk", "despair", "destroy", "detail", "detect", "develop", "device", "devote", "diagram", "dial",
	"diamond", "diary", "dice", "diesel", "diet", "differ", "digital", "dignity", "dilemma", "dinner",
	"dinosaur", "direct", "dirt", "disagree", "discover", "disease", "dish", "dismiss", "disorder", "display",
	"distance", "divert", "divide", "divorce", "dizzy", "doctor", "document", "dog", "doll", "dolphin",
	"domain", "donate", "donkey", "donor", "door", "dose", "double", "dove", "draft", "dragon",
	"drama", "draw", "dream", "dress", "drift", "drill", "drink", "drip", "drive", "drop",
	"drum", "dry", "duck", "dumb", "dune", "during", "dust", "dutch", "duty", "dwarf",
	"dynamic", "eager", "eagle", "early", "earn", "earth", "easily", "east", "easy", "echo",
	"ecology", "economy", "edge", "edit", "educate", "effort", "egg", "eight", "either", "elbow",
	"elder", "electric", "elegant", "element", "elephant", "elevator", "elite", "else", "embark", "embody",
	"embrace", "emerge", "emotion", "employ", "empower", "empty", "enable", "enact", "end", "endless",
	"endorse", "enemy", "energy", "enforce", "engage", "engine", "enhance", "enjoy", "enlist", "enough",
	"enrich", "enroll", "ensure", "enter", "entire", "entry", "envelope", "episode", "equal", "equip",
	"era", "erase", "erode", "erosion", "error", "erupt", "escape", "essay", "essence", "estate",
	"eternal", "ethics", "evidence", "evil", "evoke", "evolve", "exact", "example", "excess", "exchange",
	"excite", "exclude", "excuse", "execute", "exercise", "exhaust", "exhibit", "exile", "exist", "exit",
	"exotic", "expand", "expect", "expire", "explain", "expose", "express", "extend", "extra", "eye",
	"eyebrow", "fabric", "face", "faculty", "fade", "faint", "faith", "fall", "false", "fame",
	"family", "famous", "fan", "fancy", "fantasy", "farm", "fashion", "fat", "fatal", "father",
	"fatigue", "fault", "favorite", "feature", "february", "federal", "fee", "feed", "feel", "female",
	"fence", "festival", "fetch", "fever", "few", "fiber", "fiction", "field", "figure", "file",
	"film", "filter", "final", "find", "fine", "finger", "finish", "fire", "firm", "first",
	"fiscal", "fish", "fit", "fitness", "fix", "flag", "flame", "flash", "flat", "flavor",
	"flee", "flight", "flip", "float", "flock", "floor", "flower", "fluid", "flush", "fly",
	"foam", "focus", "fog", "foil", "fold", "follow", "food", "foot", "force", "forest",
	"forget", "fork", "fortune", "forum", "forward", "fossil", "foster", "found", "fox", "fragile",
	"frame", "frequent", "fresh", "friend", "fringe", "frog", "front", "frost", "frown", "frozen",
	"fruit", "fuel", "fun", "funny", "furnace", "fury", "future", "gadget", "gain", "galaxy",
	"gallery", "game", "gap", "garage", "garbage", "garden", "garlic", "garment", "gas", "gasp",
	"gate", "gather", "gauge", "gaze", "general", "genius", "genre", "gentle", "genuine", "gesture",
	"ghost", "giant", "gift", "giggle", "ginger", "giraffe", "girl", "give", "glad", "glance",
	"glare", "glass", "glide", "glimpse", "globe", "gloom", "glory", "glove", "glow", "glue",
	"goat", "goddess", "gold", "good", "goose", "gorilla", "gospel", "gossip", "govern", "gown",
	"grab", "grace", "grain", "grant", "grape", "grass", "gravity", "great", "green", "grid",
	"grief", "grit", "grocery", "group", "grow", "grunt", "guard", "guess", "guide", "guilt",
	"guitar", "gun", "gym", "habit", "hair", "half", "hammer", "hamster", "hand", "handle",
	"harbor", "hard", "harsh", "harvest", "hat", "have", "hawk", "hazard", "head", "health",
	"heart", "heavy", "hedgehog", "height", "hello", "helmet", "help", "hen", "hero", "hidden",
	"high", "hill", "hint", "hip", "hire", "history", "hobby", "hockey", "hold", "hole",
	"holiday", "hollow", "home", "honey", "hood", "hope", "horn", "horror", "horse", "hospital",
	"host", "hotel", "hour", "hover", "hub", "huge", "human", "humble", "humor", "hundred",
	"hungry", "hunt", "hurdle", "hurry", "hurt", "husband", "hybrid", "ice", "icon", "idea",
	"identify", "idle", "ignore", "ill", "illegal", "illness", "image", "imitate", "immense", "immune",
	"impact", "impose", "improve", "impulse", "inch", "include", "income", "increase", "index", "indicate",
	"indoor", "industry", "infant", "inflict", "inform", "inhale", "inherit", "initial", "inject", "injury",
	"inmate", "inner", "innocent", "input", "inquiry", "insane", "insect", "inside", "inspire", "install",
	"intact", "interest", "into", "invest", "invite", "involve", "iron", "island", "isolate", "issue",
	"item", "ivory", "jacket", "jaguar", "jar", "jazz", "jealous", "jeans", "jelly", "jewel",
	"job", "join", "joke", "journey", "joy", "judge", "juice", "jump", "jungle", "junior",
	"junk", "just", "kangaroo", "keen", "keep", "ketchup", "key", "kick", "kid", "kidney",
	"kind", "kingdom", "kiss", "kit", "kitchen", "kite", "kitten", "kiwi", "knee", "knife",
	"knock", "know", "lab", "label", "labor", "ladder", "lady", "lake", "lamp", "language",
	"laptop", "large", "later", "latin", "laugh", "laundry", "lava", "law", "lawn", "lawsuit",
	"layer", "lazy", "leader", "leaf", "learn", "leave", "lecture", "left", "leg", "legal",
	"legend", "leisure", "lemon", "lend", "length", "lens", "leopard", "lesson", "letter", "level",
	"liar", "liberty", "library", "license", "life", "lift", "light", "like", "limb", "limit",
	"link", "lion", "liquid", "list", "little", "live", "lizard", "load", "loan", "lobster",
	"local", "lock", "logic", "lonely", "long", "loop", "lottery", "loud", "lounge", "love",
	"loyal", "lucky", "luggage", "lumber", "lunar", "lunch", "luxury", "lyrics", "machine", "mad",
	"magic", "magnet", "maid", "mail", "main", "major", "make", "mammal", "man", "manage",
	"mandate", "mango", "mansion", "manual", "maple", "marble", "march", "margin", "marine", "market",
	"marriage", "mask", "mass", "master", "match", "material", "math", "matrix", "matter", "maximum",
	"maze", "meadow", "mean", "measure", "meat", "mechanic", "medal", "media", "melody", "melt",
	"member", "memory", "mention", "menu", "mercy", "merge", "merit", "merry", "mesh", "message",
	"metal", "method", "middle", "midnight", "milk", "million", "mimic", "mind", "minimum", "minor",
	"minute", "miracle", "mirror", "misery", "miss", "mistake", "mix", "mixed", "mixture", "mobile",
	"model", "modify", "mom", "moment", "monitor", "monkey", "monster", "month", "moon", "moral",
	"more", "morning", "mosquito", "mother", "motion", "motor", "mountain", "mouse", "move", "movie",
	"much", "muffin", "mule", "multiply", "muscle", "museum", "mushroom", "music", "must", "mutual",
	"myself", "mystery", "myth", "naive", "name", "napkin", "narrow", "nasty", "nation", "nature",
	"near", "neck", "need", "negative", "neglect", "neither", "nephew", "nerve", "nest", "net",
	"network", "neutral", "never", "news", "next", "nice", "night", "noble", "noise", "nominee",
	"noodle", "normal", "north", "nose", "notable", "note", "nothing", "notice", "novel", "now",
	"nuclear", "number", "nurse", "nut", "oak", "obey", "object", "oblige", "obscure", "observe",
	"obtain", "obvious", "occur", "ocean", "october", "odor", "off", "offer", "office", "often",
	"oil", "okay", "old", "olive", "olympic", "omit", "once", "one", "onion", "online",
	"only", "open", "opera", "opinion", "oppose", "option", "orange", "orbit", "orchard", "order",
	"ordinary", "organ", "orient", "original", "orphan", "ostrich", "other", "outdoor", "outer", "output",
	"outside", "oval", "oven", "over", "own", "owner", "oxygen", "oyster", "ozone", "pact",
	"paddle", "page", "pair", "palace", "palm", "panda", "panel", "panic", "panther", "paper",
	"parade", "parent", "park", "parrot", "party", "pass", "patch", "path", "patient", "patrol",
	"pattern", "pause", "pave", "payment", "peace", "peanut", "pear", "peasant", "pelican", "pen",
	"penalty", "pencil", "people", "pepper", "perfect", "permit", "person", "pet", "phone", "photo",
	"phrase", "physical", "piano", "picnic", "picture", "piece", "pig", "pigeon", "pilot", "pin",
	"pine", "pink", "pipe", "pistol", "pitch", "pizza", "place", "planet", "plastic", "plate",
	"play", "please", "pledge", "pluck", "plug", "plunge", "poem", "poet", "point", "polar",
	"pole", "police", "pond", "pony", "pool", "popular", "portion", "position", "possible", "post",
	"potato", "pottery", "poverty", "powder", "power", "practice", "praise", "predict", "prefer", "prepare",
	"present", "pretty", "prevent", "price", "pride", "primary", "print", "priority", "prison", "private",
	"prize", "problem", "process", "produce", "profit", "program", "project", "promote", "proof", "property",
	"prosper", "protect", "proud", "provide", "public", "pudding", "pull", "pulp", "pulse", "pumpkin",
	"punch", "pupil", "puppy", "purchase", "purity", "purpose", "purse", "push", "put", "puzzle",
	"pyramid", "quality", "quantum", "quarter", "question", "quick", "quit", "quiz", "quote", "rabbit",
	"raccoon", "race", "rack", "radar", "radio", "rail", "rain", "raise", "rally", "ramp",
	"ranch", "random", "range", "rapid", "rare", "rate", "rather", "raven", "raw", "razor",
	"ready", "real", "reason", "rebel", "rebuild", "recall", "receive", "recipe", "record", "recycle",
	"reduce", "reflect", "reform", "refuse", "region", "regret", "regular", "reject", "relax", "release",
	"relief", "rely", "remain", "remember", "remind", "remove", "render", "renew", "rent", "reopen",
	"repair", "repeat", "replace", "report", "require", "rescue", "resemble", "resist", "resource", "response",
	"result", "retire", "retreat", "return", "reunion", "reveal", "review", "reward", "rhythm", "rib",
	"ribbon", "rice", "rich", "ride", "ridge", "rifle", "right", "rigid", "ring", "riot",
	"ripple", "risk", "ritual", "rival", "river", "road", "roast", "robot", "robust", "rocket",
	"romance", "roof", "rookie", "room", "rose", "rotate", "rough", "round", "route", "royal",
	"rubber", "rude", "rug", "rule", "run", "runway", "rural", "sad", "saddle", "sadness",
	"safe", "sail", "salad", "salmon", "salon", "salt", "salute", "same", "sample", "sand",
	"satisfy", "satoshi", "sauce", "sausage", "save", "say", "scale", "scan", "scare", "scatter",
	"scene", "scheme", "school", "science", "scissors", "scorpion", "scout", "scrap", "screen", "script",
	"scrub", "sea", "search", "season", "seat", "second", "secret", "section", "security", "seed",
	"seek", "segment", "select", "sell", "seminar", "senior", "sense", "sentence", "series", "service",
	"session", "settle", "setup", "seven", "shadow", "shaft", "shallow", "share", "shed", "shell",
	"sheriff", "shield", "shift", "shine", "ship", "shiver", "shock", "shoe", "shoot", "shop",
	"short", "shoulder", "shove", "shrimp", "shrug", "shuffle", "shy", "sibling", "sick", "side",
	"siege", "sight", "sign", "silent", "silk", "silly", "silver", "similar", "simple", "since",
	"sing", "siren", "sister", "sit", "situation", "six", "size", "skate", "sketch", "ski",
	"skill", "skin", "skirt", "skull", "slab", "slam", "sleep", "slender", "slice", "slide",
	"slight", "slim", "slogan", "slot", "slow", "slush", "small", "smart", "smile", "smoke",
	"smooth", "snack", "snake", "snap", "sniff", "snow", "soap", "soccer", "social", "sock",
	"soda", "soft", "solar", "soldier", "solid", "solution", "solve", "someone", "song", "soon",
	"sorry", "sort", "soul", "sound", "soup", "source", "south", "space", "spare", "spatial",
	"spawn", "speak", "special", "speed", "spell", "spend", "sphere", "spice", "spider", "spike",
	"spin", "spirit", "split", "spoil", "sponsor", "spoon", "sport", "spot", "spray", "spread",
	"spring", "spy", "square", "squeeze", "squirrel", "stable", "stadium", "staff", "stage", "stairs",
	"stamp", "stand", "start", "state", "stay", "steak", "steel", "stem", "step", "stereo",
	"stick", "still", "sting", "stock", "stomach", "stone", "stool", "story", "stove", "strategy",
	"street", "strike", "strong", "struggle", "student", "stuff", "stumble", "style", "subject", "submit",
	"subway", "success", "such", "sudden", "suffer", "sugar", "suggest", "suit", "summer", "sun",
	"sunny", "sunset", "super", "supply", "supreme", "sure", "surface", "surge", "surprise", "surround",
	"survey", "suspect", "sustain", "swallow", "swamp", "swap", "swarm", "swear", "sweet", "swift",
	"swim", "swing", "switch", "sword", "symbol", "symptom", "syrup", "system", "table", "tackle",
	"tag", "tail", "talent", "talk", "tank", "tape", "target", "task", "taste", "tattoo",
	"taxi", "teach", "team", "tell", "ten", "tenant", "tennis", "tent", "term", "test",
	"text", "thank", "that", "theme", "then", "theory", "there", "they", "thing", "this",
	"thought", "three", "thrive", "throw", "thumb", "thunder", "ticket", "tide", "tiger", "tilt",
	"timber", "time", "tiny", "tip", "tired", "tissue", "title", "toast", "tobacco", "today",
	"toddler", "toe", "together", "toilet", "token", "tomato", "tomorrow", "tone", "tongue", "tonight",
	"tool", "tooth", "top", "topic", "topple", "torch", "tornado", "tortoise", "toss", "total",
	"tourist", "toward", "tower", "town", "toy", "track", "trade", "traffic", "tragic", "train",
	"transfer", "trap", "trash", "travel", "tray", "treat", "tree", "trend", "trial", "tribe",
	"trick", "trigger", "trim", "trip", "trophy", "trouble", "truck", "true", "truly", "trumpet",
	"trust", "truth", "try", "tube", "tuition", "tumble", "tuna", "tunnel", "turkey", "turn",
	"turtle", "twelve", "twenty", "twice", "twin", "twist", "two", "type", "typical", "ugly",
	"umbrella", "unable", "unaware", "uncle", "uncover", "under", "undo", "unfair", "unfold", "unhappy",
	"uniform", "unique", "unit", "universe", "unknown", "unlock", "until", "unusual", "unveil", "update",
	"upgrade", "uphold", "upon", "upper", "upset", "urban", "urge", "usage", "use", "used",
	"useful", "useless", "usual", "utility", "vacant", "vacuum", "vague", "valid", "valley", "valve",
	"van", "vanish", "vapor", "various", "veast", "vector", "vehicle", "vein", "vendor", "venture",
	"venue", "verb", "verify", "version", "very", "vessel", "veteran", "viable", "vibrant", "vicious",
	"victory", "video", "view", "village", "vintage", "violin", "virtual", "virus", "visa", "visit",
	"visual", "vital", "vivid", "vocal", "voice", "void", "volcano", "volume", "vote", "voyage",
	"wage", "wagon", "wait", "walk", "wall", "walnut", "want", "warfare", "warm", "warrior",
	"wash", "wasp", "waste", "water", "wave", "way", "wealth", "weapon", "wear", "weasel",
	"weather", "web", "wedding", "weekend", "weird", "welcome", "west", "wet", "whale", "what",
	"wheat", "wheel", "when", "where", "whip", "whisper", "wide", "width", "wife", "wild",
	"will", "win", "window", "wine", "wing", "wink", "winner", "winter", "wire", "wisdom",
	"wise", "wish", "witness", "wolf", "woman", "wonder", "wood", "wool", "word", "work",
	"world", "worry", "worth", "wrap", "wreck", "wrestle", "wrist", "write", "wrong", "yard",
	"year", "yellow", "you", "young", "youth", "zebra", "zero", "zone", "zoo",
}

// wordlistCache provides fast word lookup
var wordlistCache = struct {
	mu          sync.RWMutex
	wordToIdx   map[string]int
	initialized bool
}{
	wordToIdx: make(map[string]int, 2048),
}

// initWordlistCache builds the word to index cache
func initWordlistCache() {
	wordlistCache.mu.Lock()
	defer wordlistCache.mu.Unlock()

	if wordlistCache.initialized {
		return
	}

	for i, word := range bip39Wordlist {
		wordlistCache.wordToIdx[word] = i
	}
	wordlistCache.initialized = true
}

// MnemonicStrength represents mnemonic word count
type MnemonicStrength int

const (
	// Strength128 is 12 words (128 bits entropy)
	Strength128 MnemonicStrength = 12

	// Strength160 is 15 words (160 bits entropy)
	Strength160 MnemonicStrength = 15

	// Strength192 is 18 words (192 bits entropy)
	Strength192 MnemonicStrength = 18

	// Strength224 is 21 words (224 bits entropy)
	Strength224 MnemonicStrength = 21

	// Strength256 is 24 words (256 bits entropy)
	Strength256 MnemonicStrength = 24
)

// GenerateMnemonic creates a new BIP39-compatible mnemonic phrase
// Production-grade: uses crypto/rand for entropy generation
func GenerateMnemonic() (string, error) {
	return GenerateMnemonicWithStrength(Strength128)
}

// GenerateMnemonicWithStrength creates mnemonic with specified strength
func GenerateMnemonicWithStrength(strength MnemonicStrength) (string, error) {
	var entropyBits int
	switch strength {
	case Strength128:
		entropyBits = MnemonicEntropyBits128
	case Strength160:
		entropyBits = 160
	case Strength192:
		entropyBits = 192
	case Strength224:
		entropyBits = 224
	case Strength256:
		entropyBits = MnemonicEntropyBits256
	default:
		return "", fmt.Errorf("%w: %d", ErrInvalidMnemonicStrength, strength)
	}

	entropy := make([]byte, entropyBits/8)
	_, err := rand.Read(entropy)
	if err != nil {
		return "", fmt.Errorf("failed to generate entropy: %w", err)
	}

	return EntropyToMnemonic(entropy)
}

// EntropyToMnemonic converts entropy to mnemonic phrase
// BIP39 compliant: includes checksum calculation
func EntropyToMnemonic(entropy []byte) (string, error) {
	entropyBits := len(entropy) * 8
	if entropyBits < 128 || entropyBits > 256 || entropyBits%32 != 0 {
		return "", fmt.Errorf("%w: %d bits", ErrEntropyTooShort, entropyBits)
	}

	checksumBits := entropyBits / 32
	hash := sha256.Sum256(entropy)

	totalBits := entropyBits + checksumBits
	wordCount := totalBits / 11

	words := make([]string, wordCount)
	entropyBitsTotal := entropyBits

	for i := 0; i < wordCount; i++ {
		index := 0
		for j := 0; j < 11; j++ {
			bitIndex := i*11 + j
			var bit bool
			if bitIndex < entropyBitsTotal {
				byteIndex := bitIndex / 8
				bitPosition := 7 - (bitIndex % 8)
				bit = (entropy[byteIndex] & (1 << bitPosition)) != 0
			} else {
				checksumBitIndex := bitIndex - entropyBitsTotal
				bit = (hash[0] & (1 << (7 - checksumBitIndex))) != 0
			}
			if bit {
				index |= (1 << (10 - j))
			}
		}
		words[i] = bip39Wordlist[index]
	}

	return strings.Join(words, " "), nil
}

// MnemonicToEntropy converts mnemonic phrase back to entropy
// BIP39 compliant: validates checksum
func MnemonicToEntropy(mnemonic string) ([]byte, error) {
	initWordlistCache()

	words := strings.Fields(mnemonic)
	wordCount := len(words)

	if wordCount != 12 && wordCount != 15 && wordCount != 18 && wordCount != 21 && wordCount != 24 {
		return nil, fmt.Errorf("%w: %d", ErrInvalidMnemonicWordCount, wordCount)
	}

	indices := make([]int, wordCount)
	for i, word := range words {
		wordlistCache.mu.RLock()
		idx, exists := wordlistCache.wordToIdx[strings.ToLower(word)]
		wordlistCache.mu.RUnlock()

		if !exists {
			return nil, fmt.Errorf("%w: %s", ErrInvalidMnemonicWord, word)
		}
		indices[i] = idx
	}

	entropyBits := (wordCount * 11) - (wordCount * 11 / 32)
	entropyBytes := (entropyBits + 7) / 8
	entropy := make([]byte, entropyBytes)

	for i := 0; i < wordCount; i++ {
		for j := 0; j < 11; j++ {
			bitIndex := i*11 + j
			if bitIndex < entropyBits {
				if (indices[i] & (1 << (10 - j))) != 0 {
					byteIndex := bitIndex / 8
					bitPosition := 7 - (bitIndex % 8)
					entropy[byteIndex] |= (1 << bitPosition)
				}
			}
		}
	}

	hash := sha256.Sum256(entropy)
	checksumBits := entropyBits / 32

	for i := 0; i < checksumBits; i++ {
		bitIndex := entropyBits + i
		wordIndex := bitIndex / 11
		bitInWord := 10 - (bitIndex % 11)

		expectedBit := (hash[0] & (1 << (7 - i))) != 0
		actualBit := (indices[wordIndex] & (1 << bitInWord)) != 0

		if expectedBit != actualBit {
			return nil, ErrInvalidMnemonicChecksum
		}
	}

	return entropy, nil
}

// MnemonicToSeed derives seed from mnemonic using PBKDF2
// BIP39 compliant: 2048 iterations with HMAC-SHA512
func MnemonicToSeed(mnemonic, passphrase string) ([]byte, error) {
	normalizedMnemonic := strings.ToLower(strings.TrimSpace(mnemonic))
	salt := MnemonicSaltPrefix + passphrase

	seed := pbkdf2.Key([]byte(normalizedMnemonic), []byte(salt), PBKDF2Iterations, 64, sha512.New)
	return seed, nil
}

// MnemonicToSeedFast derives seed without PBKDF2 (NOT recommended for production)
// Use only for testing purposes
func MnemonicToSeedFast(mnemonic, passphrase string) []byte {
	normalizedMnemonic := strings.ToLower(strings.TrimSpace(mnemonic))
	salt := MnemonicSaltPrefix + passphrase

	h := sha512.New()
	h.Write([]byte(salt))
	h.Write([]byte(normalizedMnemonic))
	return h.Sum(nil)
}

// ValidateMnemonic validates a mnemonic phrase
// Checks: word count, word validity, and checksum
func ValidateMnemonic(mnemonic string) bool {
	_, err := MnemonicToEntropy(mnemonic)
	return err == nil
}

// ValidateMnemonicWord checks if a word is in the BIP39 wordlist
func ValidateMnemonicWord(word string) bool {
	initWordlistCache()

	wordlistCache.mu.RLock()
	defer wordlistCache.mu.RUnlock()

	_, exists := wordlistCache.wordToIdx[strings.ToLower(word)]
	return exists
}

// GetWordlist returns the complete BIP39 wordlist
func GetWordlist() []string {
	result := make([]string, len(bip39Wordlist))
	copy(result, bip39Wordlist)
	return result
}

// GetWordAtIndex returns the word at specified index
func GetWordAtIndex(index int) (string, error) {
	if index < 0 || index >= len(bip39Wordlist) {
		return "", errors.New("invalid word index")
	}
	return bip39Wordlist[index], nil
}
