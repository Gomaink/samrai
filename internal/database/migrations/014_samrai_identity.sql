UPDATE instance_settings
SET value = 'samrai',
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE key = 'instance_name'
  AND value = 'PageTurner';
