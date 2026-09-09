package media

// capabilities.go — aggregate media-capability matrix (inheritance plan
// Phase 4). Deliberately tiny: a plain struct describing what the configured
// providers can do, populated by the audio/media managers. Today it only
// feeds a startup debug log; it is kept exported so the web UI can surface
// the matrix later without another store round-trip.

// Capabilities is the aggregate capability matrix across configured
// providers. Booleans are additive: any provider offering the capability
// turns it on.
type Capabilities struct {
	// ImageInput: vision/multimodal message input is available.
	ImageInput bool `json:"image_input"`
	// ImageGen: an image-generation provider is configured.
	ImageGen bool `json:"image_gen"`
	// TTS: at least one text-to-speech provider is registered.
	TTS bool `json:"tts"`
	// STT: at least one speech-to-text provider is registered.
	STT bool `json:"stt"`
	// VideoInput: video understanding input is available.
	VideoInput bool `json:"video_input"`
	// VideoGen: a video-generation provider is configured.
	VideoGen bool `json:"video_gen"`
	// DocExtract: document extraction/parsing is available.
	DocExtract bool `json:"doc_extract"`
}

// Merge unions two capability matrices (additive aggregation).
func (c Capabilities) Merge(other Capabilities) Capabilities {
	return Capabilities{
		ImageInput: c.ImageInput || other.ImageInput,
		ImageGen:   c.ImageGen || other.ImageGen,
		TTS:        c.TTS || other.TTS,
		STT:        c.STT || other.STT,
		VideoInput: c.VideoInput || other.VideoInput,
		VideoGen:   c.VideoGen || other.VideoGen,
		DocExtract: c.DocExtract || other.DocExtract,
	}
}
