package mediahelp

import (
	"github.com/malashin/ffinfo"
	"sort"
)

func GetStream(ffprobeData *ffinfo.File, sIndex int, sType StreamType) *ffinfo.Stream {
	var streams []*ffinfo.Stream

	for _, stream := range ffprobeData.Streams {
		if stream.CodecType == string(sType) {
			streams = append(streams, &stream)
		}
	}

	sort.Slice(streams, func(i, j int) (less bool) {
		s_i := streams[i]
		s_j := streams[j]

		return s_i.Index < s_j.Index
	})

	if len(streams)-1 < sIndex {
		return nil
	}

	return streams[sIndex]
}
