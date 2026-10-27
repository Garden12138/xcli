#[cfg(test)]
mod native_gateway_contract_probe {
    use super::*;
    use std::collections::BTreeMap;

    #[tokio::test]
    async fn worklog_business_calls_through_built_gateway() {
        let mut config = crate::llm::LlmConfig {
            provider: "xcli".into(),
            base_url: std::env::var("XCLI_NATIVE_BASE_URL").unwrap(),
            model: "xcli/text-default".into(),
            api_key: std::fs::read_to_string(std::env::var("XCLI_NATIVE_TOKEN_FILE").unwrap()).unwrap().trim().into(),
            extra_headers: BTreeMap::new(),
            timeout_seconds: 10,
        };
        assert_eq!(probe(&config).await.unwrap().inference_location, "local");
        let report = crate::llm::generate_report(Some(&config), "weekly_report", "2026-10-19", "2026-10-25", &[]).await.unwrap();
        assert!(report.used_llm);
        assert_eq!(report.content, "synthetic-native-result");
        let filled = crate::llm::fill_template(Some(&config), "weekly_report", "2026-10-19", "2026-10-25", &[], "# Synthetic template").await.unwrap();
        assert!(filled.used_llm);
        assert_eq!(filled.content, "synthetic-native-result");
        config.model = "xcli/text-json".into();
        let daily = crate::llm::summarize_daily_activity(Some(&config), "2026-10-25", "Synthetic native integration evidence only").await.unwrap();
        assert_eq!(daily.task, "synthetic milestone");
        assert_eq!(daily.result.as_deref(), Some("synthetic result"));
    }
}
