package plan

import (
	"reflect"
	"strings"
	"testing"
)

// Source test_deterministic.py::test_sound_is_partitioned_not_copied.
func TestRemainingSurfaceSoundPartition(t *testing.T) {
	sync, ambient := SplitSound([]SoundEvent{{Text: "a wingbeat", Layer: "sync", Shot: 1}, {Text: "distant fire", Layer: "ambient"}})
	if !reflect.DeepEqual(sync, map[int][]string{1: {"a wingbeat"}}) || !reflect.DeepEqual(ambient, []string{"distant fire"}) {
		t.Fatal(sync, ambient)
	}
	for _, a := range ambient {
		for _, track := range sync {
			for _, s := range track {
				if a == s {
					t.Fatal("ambient was duplicated into synchronized track")
				}
			}
		}
	}
}

// Source test_deterministic.py::test_a_retention_note_does_not_double_the_article.
func TestRemainingSurfaceRetentionArticle(t *testing.T) {
	note := RetentionNote(SubjectPlan{Label: "<Subject 2>", Kind: "environment", Sources: []string{"<Picture 2>"}, Descriptor: "the corridor", Attributes: []string{"the wall torch — metal construction", "the stone wall"}})
	if strings.Contains(note, "the the") || !strings.HasPrefix(note, "the wall torch") {
		t.Fatal(note)
	}
}
