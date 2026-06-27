package importer

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// certFileDirs lists, in priority order, the per-cert directory layouts to probe
// for fullchain.pem / privkey.pem, relative to the data dir. %d is the cert id.
var certFileDirs = []string{
	filepath.Join("custom_ssl", "npm-%d"),
	filepath.Join("letsencrypt", "live", "npm-%d"),
	filepath.Join("tls", "certbot", "live", "npm-%d"),
	filepath.Join("etc", "letsencrypt", "live", "npm-%d"),
}

// findCertFiles returns the absolute fullchain/privkey paths for cert id, and
// ok=true only if both exist as a pair.
func (s *importState) findCertFiles(id int64) (cert, key string, ok bool) {
	for _, layout := range certFileDirs {
		dir := filepath.Join(s.dataDir, fmt.Sprintf(layout, id))
		c := filepath.Join(dir, "fullchain.pem")
		k := filepath.Join(dir, "privkey.pem")
		if fileExists(c) && fileExists(k) {
			return c, k, true
		}
	}
	return "", "", false
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

func (s *importState) importCertificates() error {
	want := []string{"id", "provider", "nice_name", "domain_names", "expires_on", "meta", "is_deleted"}
	cols, _, ok, err := s.selectAvailable("certificate", want)
	if err != nil {
		return err
	}
	if !ok {
		s.warn("certificate", "table", "certificate table not present; no certificates imported")
		return nil
	}
	rows, err := s.queryRows("certificate", cols)
	if err != nil {
		return err
	}

	for _, r := range rows {
		id := asInt(r["id"])
		niceName := asString(r["nice_name"])
		provider := asString(r["provider"])
		label := fmt.Sprintf("certificate #%d (%s)", id, firstNonEmpty(niceName, "unnamed"))

		domains, dok := parseDomains(asString(r["domain_names"]))
		if !dok {
			s.warn(label, "domain_names", "could not parse domain_names; certificate skipped")
			continue
		}

		name := s.uniqueName("Certificate", niceName, "cert", id)

		cert := model.Certificate{
			ObjectMeta: model.ObjectMeta{
				Name:        name,
				DisplayName: firstNonEmpty(niceName, domains[0]),
			},
			Type:    model.CertTypeCustom,
			Domains: domains,
			Custom: &model.CustomCertSpec{
				CertFile: filepath.Join(name, "fullchain.pem"),
				KeyFile:  filepath.Join(name, "privkey.pem"),
			},
		}

		certPath, keyPath, found := s.findCertFiles(id)
		if found {
			s.res.Certs = append(s.res.Certs, CertCopy{
				Name:    name,
				CertPEM: certPath,
				KeyPEM:  keyPath,
			})
		} else {
			s.warn(label, "files",
				fmt.Sprintf("certificate files not found under %s; cert must be re-supplied", s.dataDir))
		}

		if !s.add(label, "", cert) {
			continue
		}

		// Let's Encrypt certs are imported statically; renewal is lost.
		if provider == "letsencrypt" {
			s.warn(label, "provider",
				"imported as a static custom certificate; reconfigure as an ACME certificate for automatic renewal")
		}

		// Record id->name only when files exist, so hosts only reference certs
		// that will actually be present on disk.
		s.certNames[id] = name
		s.certOK[id] = found
	}
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
