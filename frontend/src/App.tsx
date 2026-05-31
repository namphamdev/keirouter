import { Routes, Route } from "react-router-dom";
import { Layout } from "./components/Layout";
import { OverviewPage } from "./pages/Overview";
import { ProvidersPage } from "./pages/Providers";
import { AccountsPage } from "./pages/Accounts";
import { ChainsPage } from "./pages/Chains";
import { KeysPage } from "./pages/Keys";
import { BudgetsPage } from "./pages/Budgets";

export function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<OverviewPage />} />
        <Route path="providers" element={<ProvidersPage />} />
        <Route path="accounts" element={<AccountsPage />} />
        <Route path="chains" element={<ChainsPage />} />
        <Route path="keys" element={<KeysPage />} />
        <Route path="budgets" element={<BudgetsPage />} />
      </Route>
    </Routes>
  );
}