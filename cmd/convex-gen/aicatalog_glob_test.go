package main

import "testing"

func TestMatchesAnyGlob(t *testing.T) {
	cases := []struct {
		path     string
		patterns []string
		want     bool
	}{
		{"admin/userAdmin/banUser", []string{"admin/**"}, true},
		{"communities/communityMutations/deleteCommunity", []string{"**/*delete*"}, true},
		{"payments/payMutations/refundCharge", []string{"payments/refund*"}, false}, // refund* only matches the leaf name
		{"payments/refundActions/refundCharge", []string{"payments/refund*"}, false},
		{"payments/refundCharge", []string{"payments/refund*"}, true},
		{"events/eventMutations/createEvent", []string{"admin/**", "moderation/**"}, false},
		{"migrations/backfill/run", []string{"migrations/**"}, true},
		// Case-insensitive: capitalized destructive verbs in leaf names must be caught.
		{"users/usersDeletion/adminDeleteUser", []string{"**/*delete*"}, true},
		{"sms/smsService/sendBulkSMS", []string{"**/*bulk*"}, true},
		{"media/mediaManagement/superAdminDeleteMedia", []string{"**/*admin*"}, true},
		// But precise patterns must not over-match: "ban" must not catch "banner".
		{"privacy/mapPrivacyBannerMutations/markMapPrivacyBannerShown", []string{"**/banUser", "**/unbanUser"}, false},
		{"users/userBanMutations/banUser", []string{"**/banUser"}, true},
	}
	for _, c := range cases {
		if got := matchesAnyGlob(c.path, c.patterns); got != c.want {
			t.Errorf("matchesAnyGlob(%q, %v) = %v, want %v", c.path, c.patterns, got, c.want)
		}
	}
}
