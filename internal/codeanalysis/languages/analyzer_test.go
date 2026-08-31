package languages

import (
	"testing"
)

func TestAnalyzeJavaSource(t *testing.T) {
	code := `package com.example;
import java.util.List;

public class OrderController {
    public void submitOrder() {
        validate();
        persist();
    }

    private void validate() {}
    private void persist() {}
}`

	analysis := AnalyzeJavaSource(code)
	if analysis.Package != "com.example" {
		t.Errorf("expected package com.example, got %s", analysis.Package)
	}
	if len(analysis.Classes) != 1 || analysis.Classes[0] != "OrderController" {
		t.Errorf("expected class OrderController, got %v", analysis.Classes)
	}
	if len(analysis.Methods) != 3 {
		t.Errorf("expected 3 methods, got %d", len(analysis.Methods))
	}
}

func TestAnalyzeRustSource(t *testing.T) {
	code := `use std::sync::Arc;

pub struct AuthToken;

impl AuthToken {
    pub fn verify() {
        check_signature();
    }
    fn check_signature() {}
}`

	analysis := AnalyzeRustSource(code)
	if len(analysis.Structs) != 1 || analysis.Structs[0] != "AuthToken" {
		t.Errorf("expected struct AuthToken, got %v", analysis.Structs)
	}
	if len(analysis.Functions) != 2 {
		t.Errorf("expected 2 functions, got %d", len(analysis.Functions))
	}
}

func TestAnalyzeCppSource(t *testing.T) {
	code := `#include <string>
namespace auth {
class Session {
    void validate() {
        checkExpiry();
    }
    void checkExpiry() {}
};
}`

	analysis := AnalyzeCppSource(code)
	if len(analysis.Namespaces) != 1 || analysis.Namespaces[0] != "auth" {
		t.Errorf("expected namespace auth, got %v", analysis.Namespaces)
	}
	if len(analysis.Classes) != 1 || analysis.Classes[0] != "Session" {
		t.Errorf("expected class Session, got %v", analysis.Classes)
	}
	if len(analysis.Functions) != 2 {
		t.Errorf("expected 2 functions, got %d", len(analysis.Functions))
	}
}
