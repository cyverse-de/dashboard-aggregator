/**
 * Gets the list of apps that have been used recently by the authenticated user.
 *
 * @module apps/recentlyUsed
 */

import { getPublicAppIDs } from "../clients/permissions";
import { validateInterval, validateLimit } from "../util";
import * as config from "../configuration";
import constants from "../constants";
import logger from "../logging";

import opentelemetry from "@opentelemetry/api";

function tracer() {
    return opentelemetry.trace.getTracer("dashboard-aggregator");
}

// All apps returned by this query are DE apps, so the system ID can be constant.
const recentlyUsedAppsQuery = `
    SELECT DISTINCT
        a.id,
        'de' AS system_id,
        a.name,
        a.description,
        a.wiki_url,
        a.integration_date,
        a.edited_date,
        a.integrator_username AS username,
        EXISTS (
            SELECT * FROM users authenticated_user
            JOIN workspace w ON authenticated_user.id = w.user_id
            JOIN app_category_group acg ON w.root_category_id = acg.parent_category_id
            JOIN app_category_app aca ON acg.child_category_id = aca.app_category_id
            WHERE authenticated_user.username = $1
            AND acg.child_index = $2
            AND aca.app_id = a.id
        ) AS is_favorite,
        (a.id = ANY ($3)) AS is_public,
        max(j.start_date) AS most_recent_start_date
    FROM jobs j
    JOIN users ju ON j.user_id = ju.id
    JOIN app_listing a ON CAST(a.id AS text) = j.app_id
    WHERE ju.username = $1
    AND NOT a.deleted
    AND NOT a.disabled
    AND j.start_date > now() - CAST($4 AS interval)
    GROUP BY a.id, a.name, a.description, a.wiki_url, a.integration_date, a.edited_date, a.integrator_username
    ORDER BY most_recent_start_date DESC
    LIMIT $5
`;

export const getRecentlyUsedApps = async (
    db,
    username,
    limit,
    startDateInterval,
    publicAppIDs
) => {
    return tracer().startActiveSpan(
        "apps/recentlyAdded getData",
        async (span) => {
            try {
                const { rows } = await db
                    .query(recentlyUsedAppsQuery, [
                        username,
                        config.favoritesGroupIndex,
                        publicAppIDs,
                        startDateInterval,
                        limit,
                    ])
                    .catch((e) => {
                        throw e;
                    });

                if (!rows) {
                    throw new Error("no rows returned");
                }

                // Remove unwanted columns from the result; doing this in SQL made the query a little clunky.
                for (const r of rows) {
                    delete r["most_recent_start_date"];
                }

                return rows;
            } finally {
                span.end();
            }
        }
    );
};

const getHandler = (db) => async (req, res) => {
    try {
        const username = req.params.username;
        const limit = validateLimit(req?.query?.limit) ?? 10;
        const startDateInterval =
            (await validateInterval(db, req?.query["start-date-interval"])) ??
            constants.DEFAULT_START_DATE_INTERVAL;
        const publicAppIDs = await getPublicAppIDs();

        // Query the database.
        const rows = await getRecentlyUsedApps(
            db,
            username,
            limit,
            startDateInterval,
            publicAppIDs
        );

        res.status(200).json({ apps: rows });
    } catch (e) {
        logger.error(e.message);
        res.status(500).json({ reason: `error running query: ${e.message}` });
    }
};

export default getHandler;
