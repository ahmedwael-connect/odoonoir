package service

import (
	"context"
	"testing"
)

func TestStartNotFound(t *testing.T) {
	svc := newTestService(t)
	if err := svc.Start(context.Background(), "nope", "", NopSink); err == nil {
		t.Fatal("expected error for missing instance")
	}
}

func TestStopNotFound(t *testing.T) {
	svc := newTestService(t)
	if err := svc.Stop(context.Background(), "nope", NopSink); err == nil {
		t.Fatal("expected error")
	}
}

func TestRestartNotFound(t *testing.T) {
	svc := newTestService(t)
	if err := svc.Restart(context.Background(), "nope", "", NopSink); err == nil {
		t.Fatal("expected error")
	}
}

func TestBrowseRecordsNotFound(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.BrowseRecords("nope", "", RecordBrowserOptions{Model: "res.partner"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestModelInfoNotFound(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.ModelInfo("nope", "", "res.partner")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCronListNotFound(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.CronList("nope", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestModuleDepGraphNotFound(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.ModuleDepGraph("nope", "", ModuleDepGraphOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateRecordNotFound(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.CreateRecord("nope", "", RecordCreateInput{Model: "res.partner"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateRecordNotFound(t *testing.T) {
	svc := newTestService(t)
	err := svc.UpdateRecord("nope", "", RecordUpdateInput{Model: "res.partner", ID: 1})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteRecordNotFound(t *testing.T) {
	svc := newTestService(t)
	err := svc.DeleteRecord("nope", "", RecordDeleteInput{Model: "res.partner", IDs: []int64{1}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListAddonPathsNotFound(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.ListAddonPaths("nope")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDetectEnterpriseNotFound(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.DetectEnterprise("nope")
	if err == nil {
		t.Fatal("expected error")
	}
}
