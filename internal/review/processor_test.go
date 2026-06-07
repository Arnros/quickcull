package review

import (
	"testing"

	"quickcull/internal/domain"
)

func TestShouldSkipEagerHEICThumbnail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ft   domain.FileType
		ctx  *ProcessorContext
		want bool
	}{
		{
			name: "skip heic when large-library mode and no cgo",
			ft:   domain.FileTypeHEIC,
			ctx:  &ProcessorContext{SkipBackgroundHash: true},
			want: !HeicSupported(),
		},
		{
			name: "do not skip heic when not in large-library mode",
			ft:   domain.FileTypeHEIC,
			ctx:  &ProcessorContext{SkipBackgroundHash: false},
			want: false,
		},
		{
			name: "do not skip non-heic",
			ft:   domain.FileTypeJPEG,
			ctx:  &ProcessorContext{SkipBackgroundHash: true},
			want: false,
		},
		{
			name: "do not skip with nil context",
			ft:   domain.FileTypeHEIC,
			ctx:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := shouldSkipEagerHEICThumbnail(tt.ft, tt.ctx)
			if got != tt.want {
				t.Fatalf("shouldSkipEagerHEICThumbnail() = %v, want %v", got, tt.want)
			}
		})
	}
}

