// The awards page Stash serves at /plugins/awards/:performerId.
//
// Data comes from the plugin backend in one call: everything stored for the
// performer, with stale sources brought up to date first. Rendering then stays
// local — switching tabs never re-fetches, and only an explicit refresh talks
// to the outside world again.
import {
  React,
  Bootstrap,
  ReactRouterDOM,
  FontAwesomeIcon,
  FontAwesomeSolid,
  useToast,
} from "./plugin";
import { getAwards, syncSource } from "./api";
import type { Award, AwardResult, AwardsPayload, SourceView } from "./types";

const { Alert, Badge, Button, Card, Spinner, Tabs, Tab } = Bootstrap;

// --- small helpers ----------------------------------------------------------

// timeAgo renders a backend timestamp the way the ADR mock does: "3 days ago".
function timeAgo(iso: string): string {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return iso;
  const seconds = Math.max(0, Math.floor((Date.now() - then) / 1000));
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return minutes === 1 ? "1 minute ago" : `${minutes} minutes ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return hours === 1 ? "1 hour ago" : `${hours} hours ago`;
  const days = Math.floor(hours / 24);
  return days === 1 ? "1 day ago" : `${days} days ago`;
}

// movieUrl makes an award's movie link followable. IAFD hands out relative
// paths, so they are resolved against the page the award was scraped from.
function movieUrl(award: Award): string | null {
  const url = award.associatedMovieUrl;
  if (!url) return null;
  if (/^https?:\/\//i.test(url)) return url;
  if (award.sourceUrl) {
    try {
      return new URL(url, award.sourceUrl).toString();
    } catch {
      // invalid base — cannot resolve, hide link instead of broken relative
    }
    return null;
  }
  // No base: only root-relative paths are safe to keep as-is.
  return url.startsWith("/") ? url : null;
}

const resultBadgeVariant: Record<AwardResult, string> = {
  won: "success",
  nominated: "secondary",
  inducted: "warning",
};

// Grouping keeps the first-appearance order of organizations, which the
// backend's "year DESC, organization ASC" sort already arranges sensibly.
function groupByOrganization(awards: Award[]) {
  const groups: { organization: string; awards: Award[] }[] = [];
  const index = new Map<string, number>();
  for (const award of awards) {
    let at = index.get(award.organization);
    if (at === undefined) {
      at = groups.length;
      index.set(award.organization, at);
      groups.push({ organization: award.organization, awards: [] });
    }
    groups[at].awards.push(award);
  }
  return groups;
}

// --- components -------------------------------------------------------------

function AwardRow({ award }: { award: Award }) {
  const url = movieUrl(award);
  return (
    <div className="awards-row">
      <span className="awards-year">{award.year}</span>
      <Badge variant={resultBadgeVariant[award.result] ?? "secondary"}>
        {award.result}
      </Badge>
      <span className="awards-name">
        {award.awardName}
        {award.category ? `: ${award.category}` : ""}
        {award.associatedMovie ? (
          <>
            {" — "}
            {url ? (
              <a href={url} target="_blank" rel="noopener noreferrer">
                {award.associatedMovie}
              </a>
            ) : (
              award.associatedMovie
            )}
            {award.associatedMovieYear ? ` (${award.associatedMovieYear})` : ""}
          </>
        ) : null}
      </span>
    </div>
  );
}

function AwardGroup({
  organization,
  awards,
}: {
  organization: string;
  awards: Award[];
}) {
  return (
    <Card className="awards-group">
      <Card.Header>{organization}</Card.Header>
      <Card.Body>
        {awards.map((award) => (
          <AwardRow key={award.id} award={award} />
        ))}
      </Card.Body>
    </Card>
  );
}

// The content of one source tab: awards, or an explanation of why there are
// none, plus the refresh control for this source.
function SourcePanel({
  source,
  onRefresh,
  refreshing,
}: {
  source: SourceView;
  onRefresh: (source: SourceView) => void;
  refreshing: boolean;
}) {
  const awards = source.awards ?? [];

  if (!source.enabled) {
    return (
      <Alert variant="secondary">
        {source.label} is turned off in the plugin settings, so its awards are
        not shown.
      </Alert>
    );
  }

  return (
    <div className="awards-source">
      <div className="awards-source-actions">
        <Button
          variant="secondary"
          size="sm"
          disabled={refreshing}
          onClick={() => onRefresh(source)}
        >
          <FontAwesomeIcon
            icon={FontAwesomeSolid.faSyncAlt}
            spin={refreshing}
          />
          {refreshing ? " Refreshing…" : " Refresh"}
        </Button>
        {source.url ? (
          <a href={source.url} target="_blank" rel="noopener noreferrer">
            view on {source.label}
          </a>
        ) : null}
      </div>

      {source.error ? (
        <Alert variant="warning">Last refresh failed: {source.error}</Alert>
      ) : null}

      {awards.length === 0 ? (
        <Alert variant="secondary">No awards found from {source.label}.</Alert>
      ) : (
        groupByOrganization(awards).map((group) => (
          <AwardGroup
            key={group.organization}
            organization={group.organization}
            awards={group.awards}
          />
        ))
      )}

      {source.lastSynced ? (
        <p className="awards-last-updated">
          Last updated: {timeAgo(source.lastSynced)}
        </p>
      ) : null}
    </div>
  );
}

// --- page -------------------------------------------------------------------

function BackButton({ onClick }: { onClick: () => void }) {
  return (
    <Button variant="secondary" size="sm" onClick={onClick}>
      <FontAwesomeIcon icon={FontAwesomeSolid.faArrowLeft} /> Back
    </Button>
  );
}

export const AwardsPage: React.FC<{
  match?: { params: { performerId: string } };
}> = ({ match }) => {
  const performerId = match?.params?.performerId ?? "";
  const history = ReactRouterDOM.useHistory();
  const toast = useToast();

  const [payload, setPayload] = React.useState<AwardsPayload | null>(null);
  const [loadError, setLoadError] = React.useState<string | null>(null);
  const [refreshing, setRefreshing] = React.useState<string | null>(null);
  // Bumping reload re-runs the load effect; used by the retry button.
  const [reload, setReload] = React.useState(0);

  React.useEffect(() => {
    if (!performerId) return;
    let cancelled = false;
    setPayload(null);
    setLoadError(null);
    getAwards(performerId)
      .then((data) => {
        if (!cancelled) setPayload(data);
      })
      .catch((err) => {
        if (!cancelled) setLoadError(err.message ?? String(err));
      });
    return () => {
      cancelled = true;
    };
  }, [performerId, reload]);

  const refresh = async (source: SourceView) => {
    if (refreshing) return;
    setRefreshing(source.source);
    try {
      await syncSource(performerId, source.source);
      setPayload(await getAwards(performerId, { sync: false }));
      toast.success(`${source.label} refreshed`);
    } catch (err: any) {
      toast.error(err.message ?? String(err));
    } finally {
      setRefreshing(null);
    }
  };

  if (!performerId) {
    return <Alert variant="danger">No performer id in the page address.</Alert>;
  }

  if (loadError) {
    return (
      <div className="awards-page">
        <BackButton onClick={() => history.push(`/performers/${performerId}`)} />
        <Alert variant="danger">
          Could not load awards: {loadError}
          <div>
            <Button size="sm" onClick={() => setReload(reload + 1)}>
              Retry
            </Button>
          </div>
        </Alert>
      </div>
    );
  }

  if (!payload) {
    return (
      <div className="awards-page">
        <BackButton onClick={() => history.push(`/performers/${performerId}`)} />
        <Spinner animation="border" role="status" />
      </div>
    );
  }

  const title = payload.performerName ?? payload.performerId;

  return (
    <div className="awards-page">
      <div className="awards-header">
        <BackButton onClick={() => history.push(`/performers/${performerId}`)} />
        <h3 className="awards-title">{title}</h3>
        <span className="awards-total">
          {payload.total} award{payload.total === 1 ? "" : "s"}
        </span>
      </div>

      {payload.warning ? <Alert variant="warning">{payload.warning}</Alert> : null}

      <Tabs defaultActiveKey={payload.sources[0]?.source} id="awards-sources">
        {payload.sources.map((source) => (
          <Tab
            key={source.source}
            eventKey={source.source}
            title={`${source.label}${source.enabled ? ` (${source.count})` : ""}`}
            mountOnEnter
            unmountOnExit={false}
          >
            <SourcePanel
              source={source}
              onRefresh={refresh}
              refreshing={refreshing === source.source}
            />
          </Tab>
        ))}
      </Tabs>
    </div>
  );
};

