package assoc

import (
	"fmt"
	"testing"
)

// TestToastNamespace derives ToastNamespace again from its URL, and checks
// the algorithm on RFC 4122's best-known version 5 UUID (www.example.com
// in the DNS namespace, as Python's uuid.uuid5 gives it).
func TestToastNamespace(t *testing.T) {
	dns := [16]byte{0x6b, 0xa7, 0xb8, 0x10, 0x9d, 0xad, 0x11, 0xd1, 0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8}
	if got, want := nameUUID(dns, "www.example.com"), "2ED6657D-E927-568B-95E1-2665A8AEA6A2"; got != want {
		t.Errorf("nameUUID(DNS, www.example.com) = %s, want %s", got, want)
	}
	url := dns
	url[3] = 0x11 // 6ba7b811-…: the URL namespace
	ns := ToastNamespace
	if got, want := nameUUID(url, "https://telaui.dev/ns/toast-activator"), fmt.Sprintf("%X-%X-%X-%X-%X", ns[0:4], ns[4:6], ns[6:8], ns[8:10], ns[10:16]); got != want {
		t.Errorf("the namespace derived from its URL is %s; ToastNamespace is %s", got, want)
	}
}

// TestToastActivatorCLSID pins the CLSIDs of a few app IDs: a change here
// breaks the registration of every installed app (the MSIX manifests and
// the registry keys written by earlier versions name the old CLSID).
func TestToastActivatorCLSID(t *testing.T) {
	for id, want := range map[string]string{
		"com.example.Notes":          "620BE956-74D9-56B1-BE2A-2D118CB430C5",
		"dev.telaui.tela-live-toast": "F17BE6C1-9B9A-5C12-A6FA-70B2171A76C5",
		"tela":                       "C98336A6-6C25-5463-9C16-AD4A175D4E74",
		"":                           "C98336A6-6C25-5463-9C16-AD4A175D4E74", // ServiceAppID("") is "tela"
	} {
		if got := ToastActivatorCLSID(id); got != want {
			t.Errorf("ToastActivatorCLSID(%q) = %s, want %s", id, got, want)
		}
	}
	if ToastActivatorCLSID("My Notes") != ToastActivatorCLSID("My-Notes") {
		t.Errorf("an ID the services change does not give the CLSID of the ID they use")
	}
	if ToastActivatorCLSID("com.example.Notes") == ToastActivatorCLSID("com.example.notes") {
		t.Errorf("app IDs that differ in case share a CLSID")
	}
}
