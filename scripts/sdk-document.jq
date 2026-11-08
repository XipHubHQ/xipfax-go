# The document the SDK is generated from: the API's own, openapi.json, with what the SDK doesn't
# offer and what New sends on every request set aside, and one form openapi-generator 7.17.0 renders
# wrong put in a form it renders right. scripts/generate.sh runs it. None of it changes what a
# request or an answer is. A document it can't read as it expects stops the generation, naming what
# it found, so that a change to the API is looked at before the SDK is made from it.

# A nullable member, as the document writes one: `anyOf` of a schema, a $ref or a type, and null.
def nullable_shape:
  type == "array"
  and length == 2
  and (map(select(. == {"type": "null"})) | length) == 1
  and (map(select(. != {"type": "null"}))[0] | type == "object" and (has("$ref") or (has("type") and .type != "null")));

# Each operation of the document, by its path and its method.
def operations(f): .paths |= map_values(map_values(if type == "object" then f else . end));

# The SDK offers the API key alone, as the TypeScript SDK does, and New sends it on every request as
# the bearer token. So the document's security is set aside: the app-token scheme, whose client
# would take an OAuth token source and need golang.org/x/oauth2, and the key's own, whose client
# would take a second key in each call's context and send it beside New's.
if (.components.securitySchemes | keys) != ["apiKey", "appToken"]
  or .components.securitySchemes.apiKey.type != "http"
  or .components.securitySchemes.apiKey.scheme != "bearer"
then
  error("openapi.json's security schemes aren't apiKey, a bearer token, and appToken: see what the SDK should offer before generating it")
else . end
| del(.components.securitySchemes, .security)
| operations(del(.security))

# New names the version the SDK is generated from in every request's Xip-Version, as the TypeScript
# SDK does, and the document takes that version alone. So the parameter is set aside: a call's own
# would send the header a second time, beside New's.
| .info.version as $version
| if (.components.parameters.XipVersion | .name != "Xip-Version" or .in != "header" or (.required // false) or .schema.enum != [$version]) then
    error("openapi.json's Xip-Version isn't an optional header taking \($version) alone: see how the SDK should offer a choice of versions before generating it")
  else . end
| operations(if has("parameters") then .parameters |= map(select(. != {"$ref": "#/components/parameters/XipVersion"})) else . end)
| del(.components.parameters.XipVersion)
| if [.. | objects | select(.name? == "Xip-Version" or ."$ref"? == "#/components/parameters/XipVersion")] != [] then
    error("openapi.json names Xip-Version somewhere other than its parameter XipVersion: see how the SDK should send it before generating it")
  else . end

# The webhooks are XipFax's requests to your server, which the generator would make into methods
# that post an event to the API. The body they carry stays typed, as the schema FaxEvent.
| del(.webhooks)

# The generator drops the description of a nullable member and keeps the description of the schema
# inside it, so the description moves in.
| walk(
    if type == "object" and has("anyOf") then
      if (.anyOf | nullable_shape | not) then
        error("openapi.json has an anyOf that isn't a schema or null, which scripts/sdk-document.jq doesn't read: \(.anyOf | tojson)")
      elif has("description") and (.anyOf | map(select(has("description"))) | length) > 0 then
        error("openapi.json describes a nullable member both outside and inside its anyOf: \(.description)")
      elif has("description") then
        .description as $description
        | .anyOf |= map(if . == {"type": "null"} then . else . + {description: $description} end)
        | del(.description)
      else . end
    else . end
  )

# Each generated file's header carries the document's description: its first paragraph alone keeps
# it to what the API is, so that a new entry in the document's changelog changes no generated file.
| .info.description |= (split("\n\n") | .[0])
