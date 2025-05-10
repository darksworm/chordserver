# chordserver

A server for retrieving guitar chord information.

## Endpoints

### Chord Endpoint
`GET /chords/{chord_name}`

Retrieves information about a specific chord by name.

Example:
```
GET /chords/Am7
```

### Fingering Endpoint
`GET /fingers/{fingering_pattern}`

Retrieves chords that match a specific fingering pattern.

Example:
```
GET /fingers/x02210
```

### Search Endpoint
`GET /search/{query}`

Searches for chords by name or fingering pattern. The endpoint automatically determines if the query is a chord name or fingering pattern based on the input.

#### Parameters
- `query`: The search term, which can be:
  - A chord name (e.g., "A", "Am", "C7")
  - A fingering pattern (e.g., "022000", "320003")

#### Response
Returns a JSON array of chord data. Each chord object includes:
- `key`: The chord key (e.g., "A", "C#")
- `suffix`: The chord type (e.g., "major", "minor", "7")
- `positions`: An array of positions/fingerings for the chord

#### Examples

Search by chord name:
```
GET /search/Am
```

Search by fingering pattern:
```
GET /search/022000
```

#### Notes
- For fingering patterns, use digits (0-9) for frets 0-9
- For frets 10 and above, use lowercase letters (a=10, b=11, etc.)
- Use 'x' or 'X' for muted strings
- If no results are found, the endpoint returns a 404 status code
