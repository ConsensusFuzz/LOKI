// Copyright (c) The Diem Core Contributors
// SPDX-License-Identifier: Apache-2.0

use crate::tests::{assert_json, new_test_context};
use serde_json::json;

#[tokio::test]
async fn test_get_ledger_info() {
    let context = new_test_context();
    let ledger_info = context.get_latest_ledger_info();
    let resp = context.get("/").await;

    let expected = json!({
        "chain_id": 4,
        "ledger_version": ledger_info.version().to_string(),
        "ledger_timestamp": ledger_info.timestamp().to_string(),
    });

    assert_eq!(expected, resp);
}

#[tokio::test]
async fn test_returns_not_found_for_the_invalid_path() {
    let context = new_test_context();
    let resp = context.expect_status_code(404).get("/invalid_path").await;
    assert_eq!(json!({"code": 404, "message": "Not Found"}), resp)
}

#[tokio::test]
async fn test_return_bad_request_if_method_not_allowed() {
    let context = new_test_context();
    let resp = context
        .expect_status_code(405)
        .post("/accounts/0x1/resources", json!({}))
        .await;

    let expected = json!({
        "code": 405,
        "message": "HTTP method not allowed",
    });

    assert_eq!(expected, resp);
}

#[tokio::test]
async fn test_health_check() {
    let context = new_test_context();
    let resp = context
        .reply(warp::test::request().method("GET").path("/-/healthy"))
        .await;
    assert_eq!(resp.status(), 200)
}

#[tokio::test]
async fn test_enable_jsonrpc_api() {
    let context = new_test_context();
    let resp = context
        .post(
            "/",
            json!({"jsonrpc": "2.0", "method": "get_metadata", "id": 1}),
        )
        .await;
    assert_eq!(resp["result"]["version"].as_u64().unwrap(), 0)
}

#[tokio::test]
async fn test_openapi_spec() {
    let context = new_test_context();
    let paths = ["/openapi.yaml", "/spec.html"];
    for path in paths {
        let req = warp::test::request().method("GET").path(path);
        let resp = context.reply(req).await;
        assert_eq!(resp.status(), 200);
    }
}

#[tokio::test]
async fn test_cors() {
    let context = new_test_context();
    let paths = ["/openapi.yaml", "/spec.html", "/", "/transactions"];
    for path in paths {
        let req = warp::test::request()
            .header("origin", "test")
            .header("access-control-headers", "any-header")
            .header("access-control-request-method", "POST")
            .method("OPTIONS")
            .path(path);
        let resp = context.reply(req).await;
        assert_eq!(resp.status(), 200);
        let cors_header = resp.headers().get("access-control-allow-origin").unwrap();
        assert_eq!(cors_header, "test");
    }
}

#[tokio::test]
async fn test_cors_forbidden() {
    let context = new_test_context();
    let paths = ["/openapi.yaml", "/spec.html", "/", "/transactions"];
    for path in paths {
        let req = warp::test::request()
            .header("origin", "test")
            .header("access-control-headers", "any-header")
            .header("access-control-request-method", "PUT")
            .method("OPTIONS")
            .path(path);
        let resp = context.reply(req).await;
        assert_eq!(resp.status(), 403);
        let err: serde_json::Value = serde_json::from_slice(resp.body()).unwrap();
        assert_json(
            err,
            json!({
              "code": 403,
              "message": "CORS request forbidden: request-method not allowed"
            }),
        )
    }
}
