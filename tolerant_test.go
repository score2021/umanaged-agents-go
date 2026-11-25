package umanagedagents

import (
	"encoding/json"
	"testing"
)

// v0.6.1: response models accept fields added to the API later (v0.5.0 failed on Session `layout`, Environment
// `session_count` and the Skill list summaries with `json: unknown field`). Unions still pick the right variant.
func TestResponsesAcceptFieldsAddedLater(t *testing.T) {
	session := `{"id":"sesn_1","type":"session","status":"idle","stop_reason":{"type":"end_turn","added_later":1},
	  "agent":{"id":"agent_1","version":1},"environment_id":"env_1","title":null,"vault_ids":[],"created_by":"u",
	  "created_at":"2026-11-25T00:00:00Z","updated_at":"2026-11-25T00:00:00Z","expires_at":null,"archived_at":null,"archived_by":null,
	  "resources":[
	    {"id":"res_1","type":"file","file_id":"file_1","mount_path":"/mnt/session/uploads/a.csv","created_at":"2026-11-25T00:00:00Z","updated_at":"2026-11-25T00:00:00Z","added_later":"x"},
	    {"type":"storage","storage_binding_id":"stb_1","sub_prefix":"","mode":"read_only","mount_path":"/mnt/storage","created_at":"2026-11-25T00:00:00Z","added_later":true}],
	  "a_field_added_later":{"nested":[1,2]}}`
	var list ResponseSessionList
	if err := json.Unmarshal([]byte(`{"data":[`+session+`],"next_page":null,"added_later":0}`), &list); err != nil {
		t.Fatalf("session list with unknown fields: %v", err)
	}
	s := list.Data[0]
	if s.Id != "sesn_1" || s.Status != "idle" {
		t.Fatalf("decoded session %+v", s)
	}
	res := s.Resources
	if len(res) != 2 || res[0].ResponseSessionListDataInnerResourcesInnerAnyOf == nil || res[1].ResponseSessionListDataInnerResourcesInnerAnyOf1 == nil {
		t.Fatalf("resources picked the wrong variants: %+v", res)
	}
	if res[0].ResponseSessionListDataInnerResourcesInnerAnyOf.FileId != "file_1" || res[1].ResponseSessionListDataInnerResourcesInnerAnyOf1.StorageBindingId != "stb_1" {
		t.Fatalf("resource fields %+v", res)
	}
	var env ResponseEnvironment
	if err := json.Unmarshal([]byte(`{"id":"env_1","name":"e","config":{"type":"cloud","added_later":1},"is_base":true,"created_by":null,"session_count":2,"available":true,"unavailable_reason":null,"archived_at":null,
	  "created_at":"2026-11-25T00:00:00Z","updated_at":"2026-11-25T00:00:00Z","added_later":"x"}`), &env); err != nil {
		t.Fatalf("environment with unknown fields: %v", err)
	}
	// Required fields are still required: tolerance is only for unknown ones.
	if err := json.Unmarshal([]byte(`{"name":"no id","added_later":1}`), &env); err == nil {
		t.Fatal("an environment without its required fields decoded")
	}
}
