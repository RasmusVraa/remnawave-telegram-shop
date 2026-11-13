package config

import "testing"

func TestPaymentsNotifyEventsRoundTrip(t *testing.T) {
	prevPaid, prevCancel := conf.paymentsNotifySendPaid, conf.paymentsNotifySendCancel
	t.Cleanup(func() { conf.paymentsNotifySendPaid, conf.paymentsNotifySendCancel = prevPaid, prevCancel })

	cases := []struct{ in, want string }{
		{"", "paid,cancel"},
		{"paid,cancel", "paid,cancel"},
		{"cancel", "cancel"},
		{" Paid ", "paid"},
		{"none", "none"},
		{"bogus", "none"},
	}
	for _, tc := range cases {
		applyPaymentsNotifyEvents(tc.in)
		if got := paymentsNotifyEventsCurrent(); got != tc.want {
			t.Errorf("%q: got %q want %q", tc.in, got, tc.want)
		}
		// Сохранённое значение читается обратно в то же самое.
		applyPaymentsNotifyEvents(paymentsNotifyEventsCurrent())
		if got := paymentsNotifyEventsCurrent(); got != tc.want {
			t.Errorf("%q round trip: got %q", tc.in, got)
		}
	}
}
