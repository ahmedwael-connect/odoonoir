package service

import "testing"

func BenchmarkTranslatePsqlError(b *testing.B) {
	raw := `FATAL:  no pg_hba.conf entry for host "127.0.0.1", user "odoo", database "test"`
	for i := 0; i < b.N; i++ {
		_ = translatePsqlError(raw, &DBError{Message: raw})
	}
}

func BenchmarkDedupe(b *testing.B) {
	// placeholder for logmon dedupe bench
	for i := 0; i < b.N; i++ {
		_ = i
	}
}
