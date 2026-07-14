UPDATE libraries
SET name = 'Main library',
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE name = 'Biblioteca principal'
  AND management_mode = 'managed';
