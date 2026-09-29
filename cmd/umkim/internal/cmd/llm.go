package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type llmBackend struct {
	Model   string `json:"model"`
	APIBase string `json:"apiBase"`
}

func llmCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "llm",
		Short: "Hub LiteLLM proxy settings",
	}
	cmd.AddCommand(llmGetCmd())
	cmd.AddCommand(llmSetCmd())
	cmd.AddCommand(llmReconcileCmd())
	backends := &cobra.Command{Use: "backends", Short: "LiteLLM backend discovery"}
	backends.AddCommand(llmBackendsSyncFromAppsCmd())
	cmd.AddCommand(backends)
	cmd.AddCommand(llmProfileCmd())
	return cmd
}

func llmBackendsSyncFromAppsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync-from-apps",
		Short: "Rebuild FleetLLM backends from Ready llama-cpp FleetApps",
		Long: `POST /llm/backends/sync-from-apps — replaces auto backends with Ready registered
llama-cpp FleetApps only (drops stale model names and removed spokes). Clears manual
backend override (backendsSource returns to auto). Run umkim llm reconcile --wait after.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Post("/llm/backends/sync-from-apps", map[string]interface{}{})
			if err != nil {
				return err
			}
			if err := mustOK(raw, code); err != nil {
				return err
			}
			return printPatchOK("llm backends sync-from-apps", raw)
		},
	}
}

func llmGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Get hub LLM settings (GET /llm)",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/llm")
			if err != nil {
				return err
			}
			if err := mustOK(raw, code); err != nil {
				return err
			}
			printJSON(raw)
			return nil
		},
	}
}

func llmSetCmd() *cobra.Command {
	var hubEnabled bool
	var publicHost string
	var hubEnabledSet bool
	var backendsFile string
	var backendFlags []string
	c := &cobra.Command{
		Use:   "set",
		Short: "Patch hub LLM settings (PATCH /llm)",
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]interface{}{}
			if hubEnabledSet {
				body["hubEnabled"] = hubEnabled
			}
			if cmd.Flags().Changed("public-host") {
				body["publicHost"] = publicHost
			}
			backends, err := llmBackendsFromFlags(backendsFile, backendFlags)
			if err != nil {
				return err
			}
			if backends != nil {
				body["backends"] = backends
			}
			if len(body) == 0 {
				return errNoPatchFields
			}
			raw, code, err := api.Patch("/llm", body)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code); err != nil {
				return err
			}
			return printPatchOK("llm", raw)
		},
	}
	c.Flags().BoolVar(&hubEnabled, "hub-enabled", false, "Enable hub LiteLLM proxy")
	c.Flags().StringVar(&publicHost, "public-host", "", "Public host for LLM API")
	c.Flags().StringVar(&backendsFile, "backends-file", "", "JSON file: array of {model, apiBase}")
	c.Flags().StringArrayVar(&backendFlags, "backend", nil, "Backend model=...,apiBase=... (repeatable)")
	c.Flags().Lookup("hub-enabled").NoOptDefVal = "true"
	c.PreRunE = func(cmd *cobra.Command, args []string) error {
		hubEnabledSet = cmd.Flags().Changed("hub-enabled")
		return nil
	}
	return c
}

func llmReconcileCmd() *cobra.Command {
	var wait bool
	var timeoutSec int
	c := &cobra.Command{
		Use:   "reconcile",
		Short: "Trigger hub LiteLLM reconcile (POST /llm/reconcile)",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Post("/llm/reconcile", map[string]interface{}{})
			if err != nil {
				return err
			}
			if err := mustOK(raw, code); err != nil {
				return err
			}
			if !wait {
				return printPatchOK("llm reconcile", raw)
			}
			deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
			for time.Now().Before(deadline) {
				got, c2, err := api.Get("/llm")
				if err != nil {
					return err
				}
				if err := mustOK(got, c2); err != nil {
					return err
				}
				var st struct {
					Phase string `json:"phase"`
				}
				_ = json.Unmarshal(got, &st)
				if st.Phase == "Ready" {
					printJSON(got)
					return nil
				}
				if st.Phase == "Failed" {
					printJSON(got)
					return fmt.Errorf("LLM reconcile failed")
				}
				time.Sleep(5 * time.Second)
			}
			return fmt.Errorf("timeout waiting for LLM phase Ready")
		},
	}
	c.Flags().BoolVar(&wait, "wait", false, "Poll until phase is Ready")
	c.Flags().IntVar(&timeoutSec, "timeout", 600, "Wait timeout in seconds")
	return c
}

func llmBackendsFromFlags(backendsFile string, backendFlags []string) ([]llmBackend, error) {
	if backendsFile == "" && len(backendFlags) == 0 {
		return nil, nil
	}
	var out []llmBackend
	if backendsFile != "" {
		raw, err := os.ReadFile(backendsFile)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("backends-file: %w", err)
		}
	}
	for _, f := range backendFlags {
		b, err := parseLLMBackendFlag(f)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

func parseLLMBackendFlag(s string) (llmBackend, error) {
	parts := strings.Split(s, ",")
	var model, apiBase string
	for _, p := range parts {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch strings.TrimSpace(kv[0]) {
		case "model":
			model = strings.TrimSpace(kv[1])
		case "apiBase":
			apiBase = strings.TrimSpace(kv[1])
		}
	}
	if model == "" || apiBase == "" {
		return llmBackend{}, fmt.Errorf("backend flag must include model=... and apiBase=...: %q", s)
	}
	return llmBackend{Model: model, APIBase: apiBase}, nil
}
