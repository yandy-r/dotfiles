package platform

import "testing"

// TestReadOSRelease_AptBase pins the codename fields apt repo_line templating
// consumes, from real-world os-release files. Codename keeps its historical
// VERSION_CODENAME-first meaning (the env cache contract); UbuntuCodename and
// the BaseDistro/BaseCodename pair resolve Ubuntu derivatives to their Ubuntu
// base and Debian hosts to the Debian tree, so docker/dropbox suites exist.
func TestReadOSRelease_AptBase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		contents       string
		codename       string
		ubuntuCodename string
		baseDistro     string
		baseCodename   string
	}{
		{
			name:           "ubuntu 24.04 noble",
			contents:       "ID=ubuntu\nID_LIKE=debian\nVERSION_ID=\"24.04\"\nVERSION_CODENAME=noble\nUBUNTU_CODENAME=noble\n",
			codename:       "noble",
			ubuntuCodename: "noble",
			baseDistro:     "ubuntu",
			baseCodename:   "noble",
		},
		{
			name:           "debian 12 bookworm",
			contents:       "ID=debian\nVERSION_ID=\"12\"\nVERSION_CODENAME=bookworm\n",
			codename:       "bookworm",
			ubuntuCodename: "bookworm",
			baseDistro:     "debian",
			baseCodename:   "bookworm",
		},
		{
			name:           "debian 13 trixie",
			contents:       "ID=debian\nVERSION_ID=\"13\"\nVERSION_CODENAME=trixie\n",
			codename:       "trixie",
			ubuntuCodename: "trixie",
			baseDistro:     "debian",
			baseCodename:   "trixie",
		},
		{
			name:           "linux mint 22.1 wilma",
			contents:       "NAME=\"Linux Mint\"\nID=linuxmint\nID_LIKE=\"ubuntu debian\"\nVERSION_ID=\"22.1\"\nVERSION_CODENAME=wilma\nUBUNTU_CODENAME=noble\n",
			codename:       "wilma",
			ubuntuCodename: "noble",
			baseDistro:     "ubuntu",
			baseCodename:   "noble",
		},
		{
			name:           "elementary os 8 circe",
			contents:       "ID=elementary\nID_LIKE=ubuntu\nVERSION_ID=\"8\"\nVERSION_CODENAME=circe\nUBUNTU_CODENAME=noble\n",
			codename:       "circe",
			ubuntuCodename: "noble",
			baseDistro:     "ubuntu",
			baseCodename:   "noble",
		},
		{
			name:           "pop os 22.04 jammy",
			contents:       "ID=pop\nID_LIKE=\"ubuntu debian\"\nVERSION_ID=\"22.04\"\nVERSION_CODENAME=jammy\nUBUNTU_CODENAME=jammy\n",
			codename:       "jammy",
			ubuntuCodename: "jammy",
			baseDistro:     "ubuntu",
			baseCodename:   "jammy",
		},
		{
			name:           "lmde 6 faye uses DEBIAN_CODENAME",
			contents:       "ID=linuxmint\nID_LIKE=debian\nVERSION_ID=\"6\"\nVERSION_CODENAME=faye\nDEBIAN_CODENAME=bookworm\n",
			codename:       "faye",
			ubuntuCodename: "faye",
			baseDistro:     "debian",
			baseCodename:   "bookworm",
		},
		{
			name:     "fedora has no apt base",
			contents: "ID=fedora\nVERSION_ID=42\n",
		},
		{
			name:     "arch has no apt base",
			contents: "ID=arch\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rel := readOSRelease(writeOSRelease(t, tt.contents))
			if rel.Codename != tt.codename {
				t.Errorf("Codename = %q, want %q", rel.Codename, tt.codename)
			}
			if rel.UbuntuCodename != tt.ubuntuCodename {
				t.Errorf("UbuntuCodename = %q, want %q", rel.UbuntuCodename, tt.ubuntuCodename)
			}
			if rel.BaseDistro != tt.baseDistro {
				t.Errorf("BaseDistro = %q, want %q", rel.BaseDistro, tt.baseDistro)
			}
			if rel.BaseCodename != tt.baseCodename {
				t.Errorf("BaseCodename = %q, want %q", rel.BaseCodename, tt.baseCodename)
			}
		})
	}
}
