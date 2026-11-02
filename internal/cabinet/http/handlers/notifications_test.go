package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// Подписка приходит из PushSubscription.toJSON() целиком, с expirationTime.
// decodeJSON отвергает незнакомые поля, и раньше каждая подписка получала 400.
func TestPushSubscribeReq_BrowserPayload(t *testing.T) {
	for _, body := range []string{
		`{"endpoint":"https://fcm.googleapis.com/fcm/send/abc","expirationTime":null,"keys":{"p256dh":"BPk","auth":"q1"}}`,
		`{"endpoint":"https://updates.push.services.mozilla.com/wpush/v2/x","expirationTime":1793635200000,"keys":{"p256dh":"BPk","auth":"q1"}}`,
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/me/push/subscribe", strings.NewReader(body))
		var req pushSubscribeReq
		if !decodeJSON(w, r, &req) {
			t.Fatalf("decode %s: status %d", body, w.Code)
		}
		if !pushEndpointOK(req.Endpoint) || req.Keys.P256dh != "BPk" || req.Keys.Auth != "q1" {
			t.Fatalf("decoded = %+v", req)
		}
	}
}
