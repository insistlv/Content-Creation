package app

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"
)

const (
	generationEventLevelInfo  = "info"
	generationEventLevelWarn  = "warn"
	generationEventLevelError = "error"
)

func generationTraceID(recordID uint) string {
	return fmt.Sprintf("gen-%d", recordID)
}

func (a *App) logGenerationEvent(recordID uint, level, stage, event, message string, metadata map[string]any) {
	if recordID == 0 {
		return
	}
	if strings.TrimSpace(level) == "" {
		level = generationEventLevelInfo
	}
	if strings.TrimSpace(stage) == "" {
		stage = GenerationStageQueued
	}
	safeMetadata := sanitizeGenerationEventMetadata(metadata)
	metadataJSON := "{}"
	if len(safeMetadata) > 0 {
		if raw, err := json.Marshal(safeMetadata); err == nil {
			metadataJSON = string(raw)
		}
	}

	entry := GenerationEventLog{
		GenerationRecordID: recordID,
		TraceID:            generationTraceID(recordID),
		Level:              level,
		Stage:              stage,
		Event:              strings.TrimSpace(event),
		Message:            strings.TrimSpace(message),
		MetadataJSON:       metadataJSON,
		CreatedAt:          time.Now(),
	}
	if err := a.db.Create(&entry).Error; err != nil {
		log.Printf("generation_event_persist_failed generation_id=%d trace_id=%s event=%s error=%v", recordID, entry.TraceID, entry.Event, err)
	}
	log.Printf("generation_event generation_id=%d trace_id=%s level=%s stage=%s event=%s message=%q metadata=%s", recordID, entry.TraceID, entry.Level, entry.Stage, entry.Event, entry.Message, metadataJSON)
}

func sanitizeGenerationEventMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	safe := make(map[string]any, len(metadata))
	for key, value := range metadata {
		key = strings.TrimSpace(key)
		if key == "" || isSensitiveGenerationMetadataKey(key) {
			continue
		}
		safe[key] = sanitizeGenerationEventMetadataValue(value)
	}
	return safe
}

func isSensitiveGenerationMetadataKey(key string) bool {
	lower := strings.ToLower(key)
	for _, marker := range []string{"authorization", "api_key", "apikey", "secret", "password", "token", "credential", "database_url"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func sanitizeGenerationEventMetadataValue(value any) any {
	switch typed := value.(type) {
	case string:
		return sanitizeGenerationEventString(typed)
	case map[string]any:
		return sanitizeGenerationEventMetadata(typed)
	case []string:
		cleaned := make([]string, 0, len(typed))
		for _, item := range typed {
			cleaned = append(cleaned, sanitizeGenerationEventString(item))
		}
		return cleaned
	case []any:
		cleaned := make([]any, 0, len(typed))
		for _, item := range typed {
			cleaned = append(cleaned, sanitizeGenerationEventMetadataValue(item))
		}
		return cleaned
	default:
		return value
	}
}

func sanitizeGenerationEventString(value string) string {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, "bearer ") ||
		strings.Contains(lower, "base64") ||
		strings.Contains(lower, "data:image") ||
		strings.Contains(lower, "data:video") ||
		strings.Contains(lower, "postgres://") {
		return "[redacted]"
	}
	return trimmed
}

func optionalUintForEvent(value *uint) any {
	if value == nil {
		return nil
	}
	return *value
}

func optionalIntPointerForEvent(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func providerHostForEvent(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return rawURL
	}
	return parsed.Host
}
