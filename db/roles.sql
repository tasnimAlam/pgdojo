-- Runs after clubdata.sql on first container start.
-- pgex_player can read and modify rows in schema cd and nothing else.
\c exercises
create role pgex_player with login password 'pgex';
grant usage on schema cd to pgex_player;
grant select, insert, update, delete on all tables in schema cd to pgex_player;
