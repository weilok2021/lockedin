INSERT INTO feeds (id, feed_url, title, site_url, source_type, category, description)
  VALUES
    (gen_random_uuid(), 'https://jvns.ca/atom.xml', 'Julia Evans',
     'https://jvns.ca', 'article', 'Engineering',
     'Approachable deep-dives on systems, debugging, and how computers work.'),
    (gen_random_uuid(), 'https://simonwillison.net/atom/everything/', 'Simon Willison',
     'https://simonwillison.net', 'article', 'AI',
     'LLM/AI experiments, tools, and notes.')
  ON CONFLICT (feed_url) DO UPDATE
  SET title       = EXCLUDED.title,
      site_url    = EXCLUDED.site_url,
      source_type = EXCLUDED.source_type,
      category    = EXCLUDED.category,
      description = EXCLUDED.description;