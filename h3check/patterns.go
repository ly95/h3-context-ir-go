package h3check

var posePatterns = []string{
	"(?i)\\b(?:fists?|hands?|arms?|legs?)\\s+(?:clenched|crossed|raised|folded|outstretched|on hips)",
	"(?i)\\b(?:in|adopting)\\s+a\\s+\\w+(?:\\s+\\w+)?\\s+(?:stance|pose|posture)\\b",
	"(?i)\\bposed?\\s+(?:for|in|with)\\b",
	"(?i)\\bmid-(?:stride|step|jump|swing|air)\\b",
	"(?i)\\b(?:looking|gazing|staring|facing)\\s+(?:at|into|toward)\\s+the\\s+camera\\b",
	"(?i)\\b(?:smiling|frowning|grimacing|scowling|shouting)\\s+(?:at|toward)\\b",
	"(?i)\\bseen\\s+from\\s+a\\s+\\w+\\s+angle\\b",
	"(?i)\\b(?:fighting|combat|battle|defensive|heroic)\\s+(?:stance|pose)\\b",
}
var sheetPatterns = []string{
	"(?i)\\b(?:turnaround|model sheet|character sheet|contact sheet|reference sheet)\\b",
	"(?i)\\b(?:panel|panels|panel grid|tile[sd]?\\s+layout|four[- ]by[- ]two|2x4|4x2)\\b",
	"\\b(?:FRONT|BACK|LEFT PROFILE|RIGHT PROFILE|LEFT 3/4 VIEW|RIGHT 3/4 VIEW|RELAXED STANDING POSE|CASUAL CONFIDENT POSE)\\b",
	"(?i)\\b(?:studio (?:grey|gray|backdrop)|seamless (?:grey|gray|white)|plain grey background|neutral grey backdrop)\\b",
	"(?i)\\b(?:multiple|several|eight|four)\\s+(?:views|angles|poses)\\s+of\\b",
}
var inferredPatterns = []string{
	"(?i)\\b(?:fighting|combat|battle|heroic|power|defensive|aggressive|ready)\\s+(?:stance|pose|posture)\\b",
	"(?i)\\b(?:determined|menacing|confident|threatening|sinister|noble|proud|anxious|nervous)\\b",
	"(?i)\\bready\\s+to\\s+\\w+",
	"(?i)\\babout\\s+to\\s+\\w+",
	"(?i)\\b(?:conveying|suggesting|implying|radiating|exuding)\\b",
	"(?i)\\bair\\s+of\\s+\\w+",
}
var stylePatterns = []string{
	"(?i)\\bstyle\\s+style\\b",
	"(?i)\\bwith\\s+(?:resembling|similar to|like)\\b",
	"(?i)\\bwith\\s+(?:cinematic|moody|dramatic|grainy|soft|warm|cold|muted|saturated)\\s*\\.",
	"\\bwith\\s*[.,]",
	",\\s*,",
	"(?i)\\bin\\s+style\\b",
	"\\bwith\\s+[A-Z][a-z]",
}

const cameraMotion = "(?i)\\b(?:zoom(?:s|ing)?\\s+(?:in|out)|push(?:es|ing)?\\s+in|pull(?:s|ing)?\\s+(?:out|back)|pan(?:s|ning)?\\s+(?:left|right)|truck(?:s|ing)?\\s+(?:left|right)|tilt(?:s|ing)?\\s+(?:up|down)|pedestal(?:s|ing)?\\s+(?:up|down)|arc(?:s|ing)?\\s+(?:around|about)|track(?:s|ing)?\\s+(?:with|alongside|behind)|holds?\\s+(?:a\\s+)?static|static\\s+shot|shakes?\\s+(?:slightly|strongly)|rolls?\\s+(?:clock|counter)|point\\s+of\\s+view|\\bPOV\\b)"
const cameraStem = "(?i)\\b(?:zoom|push|pull|pan|truck|tilt|pedestal|arc|track|static|shake|roll|drift|glide|crane|dolly)\\w*"
const cutPhrase = "(?i)the\\ camera\\ cuts\\ to|the\\ shot\\ cuts\\ to|the\\ shot\\ transitions\\ to|the\\ shot\\ changes\\ to|the\\ shot\\ switches\\ to|cross[- ]dissolve|\\bfades?\\s+(?:to|into|out)\\b|\\bwipes?\\s+(?:to|across)\\b|\\bwipe reveals\\b|\\bwipe\\b[^.]{0,20}\\breveals?\\b"
const moodPattern = "(?i)\\b(?:melanchol(?:y|ic)|wistful|nostalgic|hopeful|hopeless|uplifting|triumphant|heroic|epic|romantic|joyful|cheerful|sad|happy|tense|ominous|foreboding|sinister|menacing|eerie|haunting|mysterious|dreamy|whimsical|playful|somber|sombre|brooding|serene|tranquil|melodramatic|emotional|poignant|bittersweet|triumphal|(?:creating|evoking|conveying|suggesting|underscoring|reinforcing|heightening)\\s+(?:a|an|the)?\\s*[\\w\\s-]{0,30}?(?:mood|atmosphere|feeling|emotion|tension|sense)|(?:mood|atmosphere|emotional\\s+\\w+)\\s+of\\s+the\\s+(?:scene|video|piece))\\b"
const continuityPattern = "(?i)\\b(?:continue[sd]?|continuing|carries over|carrying over|carried over|remains audible|uninterrupted|unbroken|seamlessly)\\b"

var cameraContradictions = []string{
	"(?i)push(?:es|ing)?\\s+in\\b(?P<gap>[^.]{0,80}?)\\b(?:backward|backwards|away from|retreat)",
	"(?i)pull(?:s|ing)?\\s+(?:out|back)\\b(?P<gap>[^.]{0,80}?)\\b(?:closer|toward the subject|forward)",
	"(?i)static\\s+shot\\b(?P<gap>[^.]{0,80}?)\\b(?:pushes|pans|trucks|tilts|tracks|moves)\\b",
	"(?i)\\b(?:holds?\\s+static)\\b(?P<gap>[^.]{0,60}?)\\b(?:while|as)\\s+(?:it|the camera)\\s+(?:moves|drifts)",
}

const newActor = "(?i)\\b(?:he|she|they|him|her|them|his|hers|their|<Subject\\s+\\d+>|the\\s+(?:man|woman|figure|boy|girl|child|subject|character|dog|cat|animal|rider|runner|driver|crowd))\\b"
const sequence = "(?i)\\b(?:then|before|after|until|once|later|afterwards?|first)\\b"
