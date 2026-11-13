import { createFileRoute } from "@tanstack/react-router";
import { SharePage } from "../ui";
export const Route = createFileRoute("/s/$token")({ component: Page });
function Page() {
  const { token } = Route.useParams();
  return <SharePage token={token} />;
}
