package galcheck

import (
	"slices"
	"strings"
)

// BackendInfo describes which known_usecases a LocalAI backend supports.
// Derived from reviewing actual gRPC method implementations in each backend's
// Go/Python source code (LocalAI backend/go/ and backend/python/).
type BackendInfo struct {
	// Possible lists all usecases the backend can support.
	Possible []string
	// Default lists the conservative safe defaults for auto-approve.
	Default []string
}

// UsecaseDesc describes a single known_usecase value and its relationship
// to LocalAI's gRPC backend API.
type UsecaseDesc struct {
	// GRPCMethod is the Backend service RPC this usecase maps to.
	GRPCMethod string
	// Deprecated marks usecases that should not be added to new models.
	Deprecated bool
	// IsModifier is true when this usecase doesn't map to its own gRPC RPC
	// but modifies how another RPC behaves (e.g., vision uses Predict with images).
	IsModifier bool
	// DependsOn names the usecase(s) this modifier requires.
	DependsOn string
	// Description is a human/LLM-readable explanation.
	Description string
}

// UsecaseDescriptions maps each known_usecase string to its gRPC and semantic info.
// This helps LLMs and reviewers understand what each usecase actually means.
var UsecaseDescriptions = map[string]UsecaseDesc{
	"chat": {
		GRPCMethod:  "Predict",
		Description: "Conversational/instruction-following via the Predict RPC with chat templates. The model processes a message history and generates a response.",
	},
	"completion": {
		GRPCMethod:  "Predict",
		Description: "Text completion via the Predict RPC with a completion template. The model continues from a prompt without chat formatting.",
	},
	"edit": {
		GRPCMethod:  "Predict",
		Deprecated:  true,
		Description: "Deprecated. Text editing via the Predict RPC with an edit template (OpenAI /v1/edits, removed upstream 2024).",
	},
	"vision": {
		GRPCMethod:  "Predict",
		IsModifier:  true,
		DependsOn:   "chat",
		Description: "The model accepts images alongside text in the Predict RPC. For llama-cpp this requires an mmproj file. This is a modifier on chat/completion, not a standalone capability.",
	},
	"embeddings": {
		GRPCMethod:  "Embedding",
		Description: "Vector embedding generation via the Embedding RPC. Converts text into dense vector representations for search/retrieval.",
	},
	"tokenize": {
		GRPCMethod:  "TokenizeString",
		Description: "Tokenization via the TokenizeString RPC. Splits text into tokens without running inference.",
	},
	"image": {
		GRPCMethod:  "GenerateImage",
		Description: "Image generation via the GenerateImage RPC. Creates images from text prompts (Stable Diffusion, Flux, etc.).",
	},
	"video": {
		GRPCMethod:  "GenerateVideo",
		Description: "Video generation via the GenerateVideo RPC. Creates video clips from text prompts.",
	},
	"transcript": {
		GRPCMethod:  "AudioTranscription",
		Description: "Speech-to-text via the AudioTranscription RPC. Converts audio input to text.",
	},
	"tts": {
		GRPCMethod:  "TTS",
		Description: "Text-to-speech via the TTS RPC. Converts text to spoken audio output.",
	},
	"sound_generation": {
		GRPCMethod:  "SoundGeneration",
		Description: "Music/sound generation via the SoundGeneration RPC. Creates audio from text descriptions (not speech).",
	},
	"rerank": {
		GRPCMethod:  "Rerank",
		Description: "Document reranking via the Rerank RPC. Scores and reorders documents by relevance to a query.",
	},
	"detection": {
		GRPCMethod:  "Detect",
		Description: "Object detection via the Detect RPC. Identifies and locates objects in images with bounding boxes.",
	},
	"vad": {
		GRPCMethod:  "VAD",
		Description: "Voice activity detection via the VAD RPC. Detects speech segments in audio without transcription.",
	},
}

// BackendUsecases maps each LocalAI backend name to the usecases it supports.
// Backend names match what appears in gallery entry overrides.backend or config_file backend fields.
//
// Source of truth: actual gRPC method implementations in LocalAI's
// backend/go/ and backend/python/ directories.
var BackendUsecases = map[string]BackendInfo{
	// --- LLM / text generation backends ---
	"llama-cpp": {
		Possible: []string{"chat", "completion", "embeddings", "tokenize", "vision"},
		Default:  []string{"chat"},
	},
	"vllm": {
		Possible: []string{"chat", "completion", "embeddings", "vision"},
		Default:  []string{"chat"},
	},
	"vllm-omni": {
		Possible: []string{"chat", "completion", "image", "video", "tts", "vision"},
		Default:  []string{"chat"},
	},
	"transformers": {
		Possible: []string{"chat", "completion", "embeddings", "tts", "sound_generation"},
		Default:  []string{"chat"},
	},
	"mlx": {
		Possible: []string{"chat", "completion", "embeddings"},
		Default:  []string{"chat"},
	},
	"mlx-distributed": {
		Possible: []string{"chat", "completion", "embeddings"},
		Default:  []string{"chat"},
	},
	"mlx-vlm": {
		Possible: []string{"chat", "completion", "embeddings", "vision"},
		Default:  []string{"chat", "vision"},
	},
	"mlx-audio": {
		Possible: []string{"chat", "completion", "tts"},
		Default:  []string{"chat"},
	},

	// --- Image/video generation backends ---
	"diffusers": {
		Possible: []string{"image", "video"},
		Default:  []string{"image"},
	},
	"stablediffusion": {
		Possible: []string{"image"},
		Default:  []string{"image"},
	},
	"stablediffusion-ggml": {
		Possible: []string{"image"},
		Default:  []string{"image"},
	},

	// --- Speech-to-text backends ---
	"whisper": {
		Possible: []string{"transcript", "vad"},
		Default:  []string{"transcript"},
	},
	"faster-whisper": {
		Possible: []string{"transcript"},
		Default:  []string{"transcript"},
	},
	"whisperx": {
		Possible: []string{"transcript"},
		Default:  []string{"transcript"},
	},
	"moonshine": {
		Possible: []string{"transcript"},
		Default:  []string{"transcript"},
	},
	"nemo": {
		Possible: []string{"transcript"},
		Default:  []string{"transcript"},
	},
	"qwen-asr": {
		Possible: []string{"transcript"},
		Default:  []string{"transcript"},
	},
	"voxtral": {
		Possible: []string{"transcript"},
		Default:  []string{"transcript"},
	},
	"vibevoice": {
		Possible: []string{"transcript", "tts"},
		Default:  []string{"transcript", "tts"},
	},

	// --- TTS backends ---
	"piper": {
		Possible: []string{"tts"},
		Default:  []string{"tts"},
	},
	"kokoro": {
		Possible: []string{"tts"},
		Default:  []string{"tts"},
	},
	"coqui": {
		Possible: []string{"tts"},
		Default:  []string{"tts"},
	},
	"kitten-tts": {
		Possible: []string{"tts"},
		Default:  []string{"tts"},
	},
	"outetts": {
		Possible: []string{"tts"},
		Default:  []string{"tts"},
	},
	"pocket-tts": {
		Possible: []string{"tts"},
		Default:  []string{"tts"},
	},
	"qwen-tts": {
		Possible: []string{"tts"},
		Default:  []string{"tts"},
	},
	"faster-qwen3-tts": {
		Possible: []string{"tts"},
		Default:  []string{"tts"},
	},
	"fish-speech": {
		Possible: []string{"tts"},
		Default:  []string{"tts"},
	},
	"neutts": {
		Possible: []string{"tts"},
		Default:  []string{"tts"},
	},
	"chatterbox": {
		Possible: []string{"tts"},
		Default:  []string{"tts"},
	},
	"voxcpm": {
		Possible: []string{"tts"},
		Default:  []string{"tts"},
	},

	// --- Sound generation backends ---
	"ace-step": {
		Possible: []string{"tts", "sound_generation"},
		Default:  []string{"sound_generation"},
	},
	"acestep-cpp": {
		Possible: []string{"sound_generation"},
		Default:  []string{"sound_generation"},
	},
	"transformers-musicgen": {
		Possible: []string{"tts", "sound_generation"},
		Default:  []string{"sound_generation"},
	},

	// --- Utility backends ---
	"rerankers": {
		Possible: []string{"rerank"},
		Default:  []string{"rerank"},
	},
	"rfdetr": {
		Possible: []string{"detection"},
		Default:  []string{"detection"},
	},
	"silero-vad": {
		Possible: []string{"vad"},
		Default:  []string{"vad"},
	},
}

// allUsecases is the set of non-deprecated valid usecase strings.
var allUsecases = func() []string {
	var result []string
	for u, desc := range UsecaseDescriptions {
		if !desc.Deprecated {
			result = append(result, u)
		}
	}
	slices.Sort(result)
	return result
}()

// normalizeBackend converts backend names to the canonical form used in
// gallery entries (e.g., "llama.cpp" → "llama-cpp").
func normalizeBackend(backend string) string {
	return strings.ReplaceAll(backend, ".", "-")
}

// ValidUsecasesForBackend returns the usecases a backend can support.
// Returns all valid usecases if the backend is unknown.
func ValidUsecasesForBackend(backend string) []string {
	if info, ok := BackendUsecases[normalizeBackend(backend)]; ok {
		return info.Possible
	}
	return allUsecases
}

// DefaultUsecasesForBackend returns the conservative default usecases.
// Returns nil if the backend is unknown.
func DefaultUsecasesForBackend(backend string) []string {
	if info, ok := BackendUsecases[normalizeBackend(backend)]; ok {
		return info.Default
	}
	return nil
}

// IsKnownBackend reports whether the backend name is in the capabilities map.
func IsKnownBackend(backend string) bool {
	_, ok := BackendUsecases[normalizeBackend(backend)]
	return ok
}

// AllKnownBackends returns a sorted list of all known backend names.
func AllKnownBackends() []string {
	names := make([]string, 0, len(BackendUsecases))
	for name := range BackendUsecases {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// AllValidUsecases returns a sorted list of all valid usecase strings.
func AllValidUsecases() []string {
	return allUsecases
}
