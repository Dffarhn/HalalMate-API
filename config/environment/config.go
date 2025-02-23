package environment

import "os"

func GetOpenAIKey() string {
	return os.Getenv("OPENAI_API_KEY") // Simpan API Key di environment variable
}
