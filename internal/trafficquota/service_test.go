package trafficquota

import (
	"testing"

	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/remnawave"
)

func TestNodesTotal_sumsOnlyBypassNodes(t *testing.T) {
	series := remnawave.UserUsageSeries{Series: []remnawave.NodeUsageSeries{
		{UUID: "AAA", Total: 100},
		{UUID: "bbb", Total: 40},
		{UUID: "ccc", Total: 7},
	}}
	got := series.NodesTotal(parseNodeSet(" aaa , CCC ,"))
	if got != 107 {
		t.Fatalf("NodesTotal = %d, want 107", got)
	}
	if series.NodesTotal(parseNodeSet("")) != 0 {
		t.Fatal("пустой список нод должен давать 0")
	}
}

func TestPercent(t *testing.T) {
	cases := []struct {
		used, limit int64
		want        int
	}{{0, 100, 0}, {79, 100, 79}, {80, 100, 80}, {100, 100, 100}, {150, 100, 150}, {5, 0, 0}}
	for _, c := range cases {
		if got := percent(c.used, c.limit); got != c.want {
			t.Errorf("percent(%d,%d) = %d, want %d", c.used, c.limit, got, c.want)
		}
	}
}

func TestFormatGB(t *testing.T) {
	const gb = int64(1 << 30)
	cases := []struct {
		bytes int64
		lang  string
		want  string
	}{{6 * gb, "ru", "6 ГБ"}, {gb * 11 / 2, "ru", "5,5 ГБ"}, {gb * 11 / 2, "en", "5.5 GB"}, {30 * gb, "en", "30 GB"}}
	for _, c := range cases {
		if got := FormatGB(c.bytes, c.lang); got != c.want {
			t.Errorf("FormatGB(%d,%s) = %q, want %q", c.bytes, c.lang, got, c.want)
		}
	}
}

func TestMainLimitForTariff(t *testing.T) {
	if got := MainLimitForTariff(&database.Tariff{TrafficLimitBytes: 5 << 30}); got != 5<<30 {
		t.Fatalf("got %d", got)
	}
}
