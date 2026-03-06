//! Chronicle WASM Plugin Example: Auto Tagger (Rust)
//!
//! Demonstrates how to build a Chronicle WASM logic extension using the
//! Extism Rust PDK. This plugin:
//!
//! 1. Listens for `entity.created` hooks and auto-tags new entities.
//! 2. Exports a `roll` function that rolls dice (e.g., input "2d6+3").
//! 3. Exports an `on_message` handler for plugin-to-plugin messaging.
//!
//! Build: `cargo build --release`
//! Output: `target/wasm32-unknown-unknown/release/chronicle_auto_tagger.wasm`
//! Copy to: `dist/auto_tagger.wasm`

use extism_pdk::*;
use serde::{Deserialize, Serialize};

// ---------------------------------------------------------------------------
// Host function declarations
// ---------------------------------------------------------------------------
// These are provided by the Chronicle WASM runtime. Only functions matching
// the plugin's declared capabilities will be available.

#[host_fn]
extern "ExtismHost" {
    /// Logs a message to the Chronicle server log.
    fn chronicle_log(msg: String) -> String;

    /// Returns entity JSON by ID.
    fn get_entity(input: String) -> String;

    /// Returns all tags for a campaign.
    fn list_tags(input: String) -> String;

    /// Sets tags on an entity.
    fn set_entity_tags(input: String) -> String;

    /// Returns tags currently on an entity.
    fn get_entity_tags(input: String) -> String;
}

// ---------------------------------------------------------------------------
// Data types
// ---------------------------------------------------------------------------

/// Hook event payload from Chronicle (entity.created).
#[derive(Deserialize)]
struct HookEvent {
    #[serde(rename = "type")]
    event_type: String,
    entity_id: Option<String>,
    campaign_id: Option<String>,
}

/// Input for the get_entity host function.
#[derive(Serialize)]
struct GetEntityInput {
    entity_id: String,
}

/// Partial entity response (we only need the type slug).
#[derive(Deserialize)]
struct EntityInfo {
    #[serde(rename = "type")]
    entity_type: Option<String>,
}

/// Input for set_entity_tags host function.
#[derive(Serialize)]
struct SetEntityTagsInput {
    entity_id: String,
    tag_ids: Vec<i64>,
}

/// A tag from the list_tags response.
#[derive(Deserialize)]
struct Tag {
    id: i64,
    name: String,
}

/// Dice roll request (exported function input).
#[derive(Deserialize)]
struct RollInput {
    expression: String,
}

/// Dice roll result (exported function output).
#[derive(Serialize)]
struct RollResult {
    expression: String,
    rolls: Vec<u32>,
    modifier: i32,
    total: i32,
}

/// Plugin-to-plugin message envelope.
#[derive(Deserialize)]
struct MessageEnvelope {
    sender_ext_id: String,
    payload: serde_json::Value,
}

// ---------------------------------------------------------------------------
// Exported functions
// ---------------------------------------------------------------------------

/// Called by Chronicle when an entity.created hook fires.
/// Looks up the entity type and applies a matching tag if one exists.
#[plugin_fn]
pub fn on_hook(Json(event): Json<HookEvent>) -> FnResult<Json<serde_json::Value>> {
    let entity_id = match &event.entity_id {
        Some(id) => id.clone(),
        None => return Ok(Json(serde_json::json!({"ok": true, "skipped": "no entity_id"}))),
    };
    let campaign_id = match &event.campaign_id {
        Some(id) => id.clone(),
        None => return Ok(Json(serde_json::json!({"ok": true, "skipped": "no campaign_id"}))),
    };

    // Log what we're doing.
    let _ = unsafe {
        chronicle_log(format!("auto-tagger: processing {} for entity {}", event.event_type, entity_id))
    };

    // Look up the entity to get its type.
    let entity_json = unsafe {
        get_entity(serde_json::to_string(&GetEntityInput {
            entity_id: entity_id.clone(),
        })?)?
    };
    let entity: EntityInfo = serde_json::from_str(&entity_json)?;

    let entity_type = match entity.entity_type {
        Some(t) => t,
        None => return Ok(Json(serde_json::json!({"ok": true, "skipped": "no entity type"}))),
    };

    // List campaign tags and find one matching the entity type name.
    let tags_json = unsafe {
        list_tags(serde_json::to_string(&serde_json::json!({
            "campaign_id": campaign_id,
        }))?)?
    };
    let tags: Vec<Tag> = serde_json::from_str(&tags_json)?;

    // Find a tag whose name matches the entity type (case-insensitive).
    let matching_tag = tags.iter().find(|t| t.name.eq_ignore_ascii_case(&entity_type));

    if let Some(tag) = matching_tag {
        let _ = unsafe {
            set_entity_tags(serde_json::to_string(&SetEntityTagsInput {
                entity_id: entity_id.clone(),
                tag_ids: vec![tag.id],
            })?)?
        };

        let _ = unsafe {
            chronicle_log(format!("auto-tagger: tagged entity {} with '{}'", entity_id, tag.name))
        };

        Ok(Json(serde_json::json!({
            "ok": true,
            "tagged_with": tag.name,
        })))
    } else {
        Ok(Json(serde_json::json!({
            "ok": true,
            "skipped": format!("no tag matching type '{}'", entity_type),
        })))
    }
}

/// Roll dice using standard TTRPG notation (e.g., "2d6+3", "1d20", "4d8-1").
/// This is a pure function that doesn't need any host capabilities.
#[plugin_fn]
pub fn roll(Json(input): Json<RollInput>) -> FnResult<Json<RollResult>> {
    let expr = input.expression.trim().to_lowercase();

    // Parse NdS+M format.
    let (dice_part, modifier) = if let Some(pos) = expr.rfind('+') {
        (&expr[..pos], expr[pos + 1..].parse::<i32>().unwrap_or(0))
    } else if let Some(pos) = expr.rfind('-') {
        if pos > 0 {
            (&expr[..pos], -expr[pos + 1..].parse::<i32>().unwrap_or(0))
        } else {
            (expr.as_str(), 0)
        }
    } else {
        (expr.as_str(), 0)
    };

    let parts: Vec<&str> = dice_part.split('d').collect();
    if parts.len() != 2 {
        return Err(extism_pdk::Error::msg("invalid dice expression: expected NdS format"));
    }

    let count: u32 = parts[0].parse().unwrap_or(1);
    let sides: u32 = parts[1].parse().map_err(|_| extism_pdk::Error::msg("invalid sides"))?;

    if count == 0 || count > 100 || sides == 0 || sides > 1000 {
        return Err(extism_pdk::Error::msg("dice count must be 1-100, sides must be 1-1000"));
    }

    // Simple pseudo-random using WASM-safe method.
    // In production, plugins would use wasi random or a seed from config.
    let mut rolls = Vec::with_capacity(count as usize);
    let mut seed: u64 = 42; // Deterministic for example; real plugins use WASI random.
    for _ in 0..count {
        seed = seed.wrapping_mul(6364136223846793005).wrapping_add(1442695040888963407);
        let roll = ((seed >> 33) as u32 % sides) + 1;
        rolls.push(roll);
    }

    let sum: i32 = rolls.iter().map(|&r| r as i32).sum::<i32>() + modifier;

    Ok(Json(RollResult {
        expression: input.expression,
        rolls,
        modifier,
        total: sum,
    }))
}

/// Handler for plugin-to-plugin messages (requires "message" capability).
#[plugin_fn]
pub fn on_message(Json(envelope): Json<MessageEnvelope>) -> FnResult<Json<serde_json::Value>> {
    let _ = unsafe {
        chronicle_log(format!(
            "auto-tagger: received message from {}",
            envelope.sender_ext_id
        ))
    };

    Ok(Json(serde_json::json!({
        "ok": true,
        "received_from": envelope.sender_ext_id,
    })))
}
