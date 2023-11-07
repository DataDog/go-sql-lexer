CREATE OR REPLACE FUNCTION dynamic_query(sql_query text) RETURNS SETOF RECORD AS $func$
BEGIN
  RETURN QUERY EXECUTE sql_query;
END;
$func$ LANGUAGE plpgsql;