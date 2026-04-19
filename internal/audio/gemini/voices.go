package gemini

import "github.com/nextlevelbuilder/goclaw/internal/audio"

// geminiVoices is the static catalog of 30 Gemini prebuilt voices.
// Source: https://ai.google.dev/gemini-api/docs/speech-generation (April 2026)
var geminiVoices = []audio.VoiceOption{
	{VoiceID: "Zephyr", Name: "Zephyr"},
	{VoiceID: "Puck", Name: "Puck"},
	{VoiceID: "Charon", Name: "Charon"},
	{VoiceID: "Kore", Name: "Kore"},
	{VoiceID: "Fenrir", Name: "Fenrir"},
	{VoiceID: "Leda", Name: "Leda"},
	{VoiceID: "Orus", Name: "Orus"},
	{VoiceID: "Aoede", Name: "Aoede"},
	{VoiceID: "Callirrhoe", Name: "Callirrhoe"},
	{VoiceID: "Autonoe", Name: "Autonoe"},
	{VoiceID: "Enceladus", Name: "Enceladus"},
	{VoiceID: "Iapetus", Name: "Iapetus"},
	{VoiceID: "Umbriel", Name: "Umbriel"},
	{VoiceID: "Algieba", Name: "Algieba"},
	{VoiceID: "Despina", Name: "Despina"},
	{VoiceID: "Erinome", Name: "Erinome"},
	{VoiceID: "Algenib", Name: "Algenib"},
	{VoiceID: "Rasalgethi", Name: "Rasalgethi"},
	{VoiceID: "Laomedeia", Name: "Laomedeia"},
	{VoiceID: "Achernar", Name: "Achernar"},
	{VoiceID: "Alnilam", Name: "Alnilam"},
	{VoiceID: "Schedar", Name: "Schedar"},
	{VoiceID: "Gacrux", Name: "Gacrux"},
	{VoiceID: "Pulcherrima", Name: "Pulcherrima"},
	{VoiceID: "Achird", Name: "Achird"},
	{VoiceID: "Zubenelgenubi", Name: "Zubenelgenubi"},
	{VoiceID: "Vindemiatrix", Name: "Vindemiatrix"},
	{VoiceID: "Sadachbia", Name: "Sadachbia"},
	{VoiceID: "Sadaltager", Name: "Sadaltager"},
	{VoiceID: "Sulafat", Name: "Sulafat"},
}

// defaultVoice is the voice used when none is specified.
const defaultVoice = "Kore"

// validVoiceSet is a fast-lookup set built from geminiVoices at init time.
var validVoiceSet map[string]struct{}

func init() {
	validVoiceSet = make(map[string]struct{}, len(geminiVoices))
	for _, v := range geminiVoices {
		validVoiceSet[v.VoiceID] = struct{}{}
	}
}

// isValidVoice reports whether name is a known Gemini prebuilt voice.
func isValidVoice(name string) bool {
	_, ok := validVoiceSet[name]
	return ok
}
