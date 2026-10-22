package h3check

import (
	"regexp"
	"sort"
	"strings"
)

var framingGroups = map[string][]string{
	"extreme-close": {"extreme close", "macro"},
	"close":         {"close-up", "close up", "close framing", "close shot", "tight on", "chest height", "head and shoulders"},
	"medium":        {"medium shot", "medium framing", "medium wide", "waist"},
	"wide":          {"wide", "extreme wide", "establishing", "full body", "long shot"},
	"over-shoulder": {"over the shoulder", "over his shoulder"},
	"low":           {"low angle", "low-angle", "from below"},
	"high":          {"high angle", "high-angle", "overhead", "from above", "aerial"},
	"pov":           {"pov", "point of view"},
}
var actionGroups = map[string][]string{
	"locomotion": {"walk", "stride", "step", "advance", "march", "run", "jog", "approach", "recede", "move", "cross", "traverse", "head toward", "come", "proceed", "make his way", "close the distance"},
	"stopping":   {"stop", "halt", "settle", "pause", "stand still", "come to rest"},
	"turning":    {"turn", "pivot", "spin", "rotate", "face", "swivel"},
	"looking":    {"look", "glance", "gaze", "stare", "watch", "regard", "eye"},
	"reaching":   {"reach", "lift", "raise", "extend", "grab", "grasp", "take", "pick up"},
	"speaking":   {"speak", "say", "talk", "utter", "murmur", "shout"},
	"vertical":   {"climb", "descend", "rise", "fall", "kneel", "sit", "stand up", "crouch"},
	"threshold":  {"enter", "emerge", "appear", "exit", "leave", "vanish", "step into", "step out"},
	"gesture":    {"gesture", "point", "wave", "nod", "shrug", "clench", "flex"},
}
var cameraGroups = map[string][]string{
	"in":       {"push", "dolly in", "zoom in", "zooms in", "moves closer", "closes in", "approaches"},
	"out":      {"pull out", "pulls out", "pulling out", "zoom out", "zooms out", "pulls back"},
	"lateral":  {"truck", "trucks", "pan", "pans", "panning", "slide", "glides sideways"},
	"vertical": {"tilt", "tilts", "pedestal", "rises", "lowers", "cranes"},
	"orbit":    {"arc", "arcs", "orbits", "circles"},
	"follow":   {"track", "tracks", "follows", "trails"},
	"held":     {"static", "holds", "held", "locked off", "motionless"},
	"shake":    {"shake", "shakes", "judder", "vibrat"},
	"roll":     {"roll", "rolls"},
}
var mediumBuckets = map[string][]string{
	"2d-animation": {"anime", "manga", "2d animation", "hand-drawn", "cel-shaded", "cel shaded", "cartoon", "toon", "comic book", "comic illustration", "comic", "graphic novel", "illustrated animation", "illustration", "illustrated", "line art", "inked", "vector art"},
	"3d-animation": {"3d cg", "3d animation", "3d computer animation", "cgi", "pixar", "video game cutscene", "game cinematic", "rendered in 3d", "unreal engine"},
	"live-action":  {"live-action", "live action", "photographic", "photoreal", "photorealistic", "documentary", "found footage", "cinematic live"},
	"stop-motion":  {"stop-motion", "stop motion", "claymation", "clay animation", "puppet", "papercraft", "paper cut-out"},
	"painted":      {"watercolour", "watercolor", "oil painting", "gouache", "ink wash", "storybook", "painterly", "pastel drawing", "charcoal"},
	"archival":     {"vintage film", "super 8", "16mm", "vhs", "black and white film", "newsreel"},
}
var verbOnly = map[string]bool{"close": true, "come": true, "cross": true, "eye": true, "face": true, "fall": true, "head": true, "point": true, "regard": true, "rise": true, "run": true, "settle": true, "stand": true, "watch": true, "wave": true}

func groups(s string, m map[string][]string) string {
	low := strings.ToLower(s)
	var a []string
	for k, terms := range m {
		for _, t := range terms {
			hit := false
			if strings.Contains(t, " ") {
				hit = strings.Contains(low, t)
			} else if verbOnly[t] {
				hit = has(`\b`+regexp.QuoteMeta(t)+`(?:s|es|ed|ing)\b`, low)
			} else {
				hit = has(`\b`+regexp.QuoteMeta(t), low)
			}
			if hit {
				a = append(a, k)
				break
			}
		}
	}
	sort.Strings(a)
	return strings.Join(a, ",")
}
func signature(s string) [3]string {
	return [3]string{groups(s, framingGroups), groups(s, actionGroups), groups(s, cameraGroups)}
}
func classifyMedium(s string) string {
	low := strings.ToLower(s)
	best := len(low) + 1
	bucket := ""
	for k, terms := range mediumBuckets {
		for _, t := range terms {
			if i := strings.Index(low, t); i >= 0 && (i < best || (i == best && k < bucket)) {
				best, bucket = i, k
			}
		}
	}
	return bucket
}
