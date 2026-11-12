# Reference: menu JSON schema

A menu is one JSON document. This is what you send to the bot and, after
validation, exactly what is published as `static/menus/<id>.json`.

A machine-readable [JSON Schema](https://json-schema.org/) (draft 2020-12)
is in [menu.schema.json](menu.schema.json). Associate it with your file in
your editor's settings to get completion and inline errors; do not add a
`$schema` field to the menu itself, because the bot rejects unknown
fields.

## Example

```json
{
  "categories": [
    {
      "id": "starters",
      "name": "Starters",
      "description": "Something to share",
      "items": [
        {
          "name": "Bruschetta",
          "description": "Toasted bread, tomato, basil",
          "price": 6.5,
          "image": "https://example.com/bruschetta.jpg"
        },
        { "name": "Soup of the day", "price": 5 }
      ]
    }
  ]
}
```

`/template` sends a longer example.

## Menu

| Field | Type | Required | Rules |
|---|---|---|---|
| `categories` | array of [Category](#category) | yes | 1 to 50 entries |

## Category

| Field | Type | Required | Rules |
|---|---|---|---|
| `id` | string | yes | 1 to 64 characters: lowercase `a-z`, `0-9`, single hyphens between them (`main-dishes`). Unique within the menu. |
| `name` | string | yes | 1 to 100 characters |
| `description` | string | no | Up to 500 characters |
| `items` | array of [Item](#item) | yes | 0 to 200 entries; may be `[]` |

## Item

| Field | Type | Required | Rules |
|---|---|---|---|
| `name` | string | yes | 1 to 100 characters |
| `price` | number | yes | 0 to 1000000. A number, not a string: `9.5`, not `"9.50"` or `"9,50 €"`. |
| `description` | string | no | Up to 500 characters |
| `image` | string | no | Up to 2048 characters. Either an `https://` URL with a host and no credentials, or a site-relative path starting with `/` and without `..` (`/images/soup.jpg`). Plain ASCII, no spaces or backslashes. |

## Rules for the whole document

- **Unknown fields are rejected** at every level, so a typo such as
  `"desciption"` is reported instead of silently ignored.
- Text must be valid UTF-8, must not start or end with whitespace, and
  must not contain control characters (including line breaks) or
  text-direction override characters.
- Lengths are counted in characters, not bytes.
- Nothing may follow the closing `}`.
- The whole submission must fit in `MAX_UPLOAD_BYTES` (256 KB by default).
- Prices carry no currency; the site decides how to display them.

The published file is the bot's own re-encoding of what it parsed
(two-space indentation, a fixed field order), not your bytes.

## Error reports

Each problem is reported with the path of the field, what is wrong and
how to fix it, at most five per reply:

| Path | Meaning |
|---|---|
| `categories` | The top-level array |
| `categories[0].name` | `name` of the first category (indices start at 0) |
| `categories[2].items[1].price` | `price` of the second item of the third category |
| `line 7` or `line 7, column 12` | A JSON syntax error, or a line of the list format |

## The list format

For people without a JSON editor, the bot also accepts plain text and
converts it to the document above:

```
CATEGORY: Starters
DESCRIPTION: Something to share

- ITEM: Bruschetta
  PRICE: 6.50
  DESCRIPTION: Toasted bread, tomato, basil

- ITEM: Soup of the day
  PRICE: 5

CATEGORY: Desserts
- ITEM: Tiramisu
  PRICE: 6
```

- One `KEYWORD: value` per line. Keywords are `CATEGORY`, `ITEM`, `PRICE`
  and `DESCRIPTION`, in any letter case.
- Blank lines, indentation and the leading `-` are optional.
- `DESCRIPTION` applies to the item above it, or to the category if no
  item has started yet.
- Every `ITEM` needs a `PRICE`: a number with a dot for decimals and no
  currency symbol.
- Category ids are derived from the names (`Main dishes` becomes
  `main-dishes`).
- Images cannot be set in the list format; use JSON.

All the limits above apply. Problems are reported by line number.
