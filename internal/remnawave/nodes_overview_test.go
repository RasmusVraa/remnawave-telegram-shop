package remnawave

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Ответ живой ноды Remnawave 3.4: system и versions заполнены, плагин назначен.
const nodeOnlineJSON = `{
 "uuid":"6D3EB5F4-1B6B-47F1-B813-BB155A845F11","name":" Швеция2 ","address":"179.254.109.151","countryCode":"se",
 "isConnected":true,"isConnecting":false,"isDisabled":false,"isTrafficTrackingActive":true,"trafficResetDay":25,
 "trafficLimitBytes":0,"trafficUsedBytes":700000000000,"xrayUptime":345600,"usersOnline":26,
 "activePluginUuid":"AAAAAAAA-0000-0000-0000-000000000001",
 "provider":{"name":"1cent.host","faviconLink":"https://1cent.host/favicon.ico"},
 "versions":{"xray":"26.7.28","node":"3.0.0"},
 "system":{"info":{"memoryTotal":1000,"cpus":2},"stats":{"memoryFree":570,"memoryUsed":430,"uptime":10,"loadAvg":[0.13,0.21,0.15],
  "interface":{"interface":"eth0","rxBytesPerSec":1000,"txBytesPerSec":2000,"rxTotal":1,"txTotal":2}}}}`

// Нода, которую панель не опросила: system, versions, provider — null.
const nodeOfflineJSON = `{
 "uuid":"b","name":"Финка","address":"11","countryCode":"FI","isConnected":false,"isConnecting":false,"isDisabled":false,
 "isTrafficTrackingActive":false,"trafficResetDay":1,"trafficLimitBytes":0,"trafficUsedBytes":0,
 "activePluginUuid":null,"provider":null,"xrayUptime":0,"usersOnline":0,"system":null,"versions":null}`

func decodeNode(t *testing.T, raw string) nodeOverviewWire {
	t.Helper()
	var n nodeOverviewWire
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return n
}

func TestOverviewFromWire_online(t *testing.T) {
	plugins := map[string]string{"aaaaaaaa-0000-0000-0000-000000000001": "TBlocker"}
	got := overviewFromWire(decodeNode(t, nodeOnlineJSON), plugins)

	if got.State != NodeStateOnline || got.UsersOnline != 26 {
		t.Fatalf("state/users: %+v", got)
	}
	if got.UUID != "6d3eb5f4-1b6b-47f1-b813-bb155a845f11" || got.Name != "Швеция2" || got.CountryCode != "SE" {
		t.Fatalf("identity not normalized: %+v", got)
	}
	if got.PluginName != "TBlocker" {
		t.Fatalf("plugin = %q", got.PluginName)
	}
	if got.MemoryUsedPct == nil || *got.MemoryUsedPct != 43 {
		t.Fatalf("memory pct = %v", got.MemoryUsedPct)
	}
	if got.RxBytesPerSec == nil || *got.RxBytesPerSec != 1000 || *got.TxBytesPerSec != 2000 {
		t.Fatalf("speeds: %v %v", got.RxBytesPerSec, got.TxBytesPerSec)
	}
	if got.CPUs != 2 {
		t.Fatalf("cpus = %d", got.CPUs)
	}
	if got.TrafficResetDay != 25 || got.XrayUptimeSec != 345600 {
		t.Fatalf("traffic/uptime: %+v", got)
	}
	if got.XrayVersion != "26.7.28" || got.NodeVersion != "3.0.0" {
		t.Fatalf("versions: %+v", got)
	}
	if got.Provider == nil || got.Provider.Name != "1cent.host" {
		t.Fatalf("provider: %+v", got.Provider)
	}
}

func TestOverviewFromWire_offlineHasNoLiveData(t *testing.T) {
	got := overviewFromWire(decodeNode(t, nodeOfflineJSON), nil)

	if got.State != NodeStateOffline {
		t.Fatalf("state = %q", got.State)
	}
	if got.MemoryUsedPct != nil || got.RxBytesPerSec != nil || got.TxBytesPerSec != nil || got.LoadAvg != nil {
		t.Fatalf("offline node must not carry live metrics: %+v", got)
	}
	if got.Provider != nil || got.PluginName != "" {
		t.Fatalf("provider/plugin must be empty: %+v", got)
	}
	// Отслеживание трафика выключено — день сброса из панели игнорируем.
	if got.TrafficResetDay != 0 {
		t.Fatalf("reset day = %d, want 0", got.TrafficResetDay)
	}
}

func TestOverviewState_priority(t *testing.T) {
	// Отключённая вручную важнее «подключается», а «подключается» важнее «в сети».
	cases := []struct {
		n    nodeOverviewWire
		want string
	}{
		{nodeOverviewWire{IsDisabled: true, IsConnecting: true, IsConnected: true}, NodeStateDisabled},
		{nodeOverviewWire{IsConnecting: true, IsConnected: true}, NodeStateConnecting},
		{nodeOverviewWire{IsConnected: true}, NodeStateOnline},
		{nodeOverviewWire{}, NodeStateOffline},
	}
	for _, c := range cases {
		if got := overviewState(c.n); got != c.want {
			t.Errorf("overviewState(%+v) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestOverviewFromWire_pluginUnknownUUID(t *testing.T) {
	// Плагин назначен, но в списке панели его нет — имени нет, показывать нечего.
	got := overviewFromWire(decodeNode(t, nodeOnlineJSON), map[string]string{})
	if got.PluginName != "" {
		t.Fatalf("plugin = %q, want empty", got.PluginName)
	}
}

func TestOverviewFromWire_viewPosition(t *testing.T) {
	got := overviewFromWire(decodeNode(t, `{"uuid":"a","name":"x","viewPosition":7}`), nil)
	if got.ViewPosition != 7 {
		t.Fatalf("view position = %d, want 7", got.ViewPosition)
	}
}

func TestListNodesOverview_sortedByViewPosition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"response":[
			{"uuid":"c","name":"Обход","viewPosition":3},
			{"uuid":"a","name":"Финка","viewPosition":1},
			{"uuid":"b","name":"германия","viewPosition":2}]}`)
	}))
	defer srv.Close()

	nodes, err := NewClient(srv.URL, "t", "remote").ListNodesOverview(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var names []string
	for _, n := range nodes {
		names = append(names, n.Name)
	}
	if len(names) != 3 || names[0] != "Финка" || names[1] != "германия" || names[2] != "Обход" {
		t.Fatalf("order = %v", names)
	}
}

func TestReorderNodes_body(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody struct {
		Nodes []struct {
			UUID         string `json:"uuid"`
			ViewPosition int    `json:"viewPosition"`
		} `json:"nodes"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = io.WriteString(w, `{"response":[]}`)
	}))
	defer srv.Close()

	if err := NewClient(srv.URL, "t", "remote").ReorderNodes(context.Background(), []string{"b", "c", "a"}); err != nil {
		t.Fatalf("reorder: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/nodes/actions/reorder" {
		t.Fatalf("request = %s %s", gotMethod, gotPath)
	}
	if len(gotBody.Nodes) != 3 {
		t.Fatalf("body = %+v", gotBody)
	}
	// Позиции идут с 1 в порядке списка: первая в списке — первая в панели.
	for i, want := range []string{"b", "c", "a"} {
		if gotBody.Nodes[i].UUID != want || gotBody.Nodes[i].ViewPosition != i+1 {
			t.Fatalf("item %d = %+v, want %s at %d", i, gotBody.Nodes[i], want, i+1)
		}
	}
}
