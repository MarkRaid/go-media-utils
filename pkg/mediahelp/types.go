package mediahelp

import "github.com/malashin/ffinfo"

type StreamType string

const (
	StreamAny      StreamType = ""
	StreamVideo               = "video"
	StreamAudio               = "audio"
	StreamSubtitle            = "subtitle"
)

func IsVideoFile(ffprobeData *ffinfo.File) bool {
	for _, stream := range ffprobeData.Streams {
		if stream.CodecType == string(StreamVideo) {
			return true
		}
	}

	return false
}
