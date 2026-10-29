package database

import (
	"errors"
	"reflect"
	"testing"
)

func TestNormalizeBroadcastAudiences(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "один сегмент", in: []string{"active_paid"}, want: []string{"active_paid"}},
		{
			name: "порядок канонический",
			in:   []string{"inactive_paid", "active_trial"},
			want: []string{"active_trial", "inactive_paid"},
		},
		{
			name: "повторы и пробелы",
			in:   []string{" active_trial ", "active_trial", ""},
			want: []string{"active_trial"},
		},
		{
			// «Все активные» уже содержат оплативших — второй сегмент ничего не добавляет.
			name: "дочерний под родителем",
			in:   []string{"active_paid", "active_all", "inactive_paid"},
			want: []string{"active_all", "inactive_paid"},
		},
		{
			name: "всё поглощает «Все»",
			in:   []string{"inactive_trial", "all", "active_all"},
			want: []string{"all"},
		},
		{
			name: "устаревшие active / inactive",
			in:   []string{"active", "inactive"},
			want: []string{"active_all", "inactive_all"},
		},
		{name: "тест один", in: []string{"test_broadcast"}, want: []string{"test_broadcast"}},
		{name: "пусто", in: nil, want: []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeBroadcastAudiences(tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNormalizeBroadcastAudiencesErrors(t *testing.T) {
	if _, err := NormalizeBroadcastAudiences([]string{"active_all", "vip"}); err == nil {
		t.Fatal("unknown audience must fail")
	}
	// Даже под «Все» тест не прячется: иначе «проверка» ушла бы всем.
	_, err := NormalizeBroadcastAudiences([]string{"all", "test_broadcast"})
	if !errors.Is(err, ErrBroadcastTestCombined) {
		t.Fatalf("got %v, want ErrBroadcastTestCombined", err)
	}
}
