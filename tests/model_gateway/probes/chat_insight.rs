#[cfg(test)]
mod native_gateway_contract_probe {
    use super::*;

    #[test]
    fn chat_insight_privacy_sse_and_vectors_through_built_gateway() {
        let token = std::fs::read_to_string(std::env::var("XCLI_NATIVE_TOKEN_FILE").unwrap()).unwrap();
        let mut settings = AiSettings {
            provider: AiProvider::Xcli,
            enabled: true,
            base_url: std::env::var("XCLI_NATIVE_BASE_URL").unwrap(),
            chat_model: "xcli/text-default".into(),
            embed_model: "xcli/embed-default".into(),
            allow_cloud: true,
            ..AiSettings::default()
        };
        let mut client = OpenAiCompatClient::new(&settings, Some(token.trim().into()));
        let privacy = client.gateway_privacy().unwrap().unwrap();
        assert!(!privacy.chat_cloud && !privacy.embedding_cloud);
        let messages = [ChatMessage::user("Synthetic native integration input")];
        assert_eq!(client.chat(&messages, &mut |_| true).unwrap(), "synthetic-native-result");
        assert!(client.chat(&messages, &mut |_| false).is_err());
        assert!(client.embed(&["Synthetic unapproved index".into()]).is_err());
        let identity = client.prepare_index(true).unwrap().unwrap();
        assert_eq!(identity.dimensions, 3);
        assert_eq!(identity.fingerprint.len(), 64);
        let vectors = client.embed(&["Synthetic one".into(), "Synthetic two".into()]).unwrap();
        assert_eq!(vectors.len(), 2);
        assert!(vectors.iter().all(|vector| vector.len() == 3 && vector.iter().all(|value| value.is_finite())));
        settings.chat_model = "xcli/text-cloud".into();
        let remote = OpenAiCompatClient::new(&settings, Some(token.trim().into()));
        assert!(remote.gateway_privacy().unwrap().unwrap().chat_cloud);
        assert!(remote.chat(&messages, &mut |_| true).is_err());
        let remote = remote.with_cloud_consent(true);
        assert_eq!(remote.chat(&messages, &mut |_| true).unwrap(), "synthetic-native-result");
    }
}
