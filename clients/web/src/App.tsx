import { BrowserRouter, Routes, Route, Navigate } from 'react-router';
import { ClientProvider } from '@/hooks/useConstellClient';
import { AuthGuard } from '@/components/auth/AuthGuard';
import LoginPage from '@/pages/LoginPage';
import RegisterPage from '@/pages/RegisterPage';
import MainPage from '@/pages/MainPage';

export default function App() {
  return (
    <ClientProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/register" element={<RegisterPage />} />
          <Route
            element={
              <AuthGuard>
                <MainPage />
              </AuthGuard>
            }
          >
            <Route path="/@me" element={<div className="flex flex-1 items-center justify-center text-[#585b70]">Select a conversation</div>} />
            <Route path="/@me/:peerId" element={<div>DM Chat placeholder</div>} />
            <Route path="/:communityId" element={<div>Channel placeholder</div>} />
            <Route path="/:communityId/:channelId" element={<div>Channel chat placeholder</div>} />
            <Route path="/" element={<Navigate to="/@me" replace />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </ClientProvider>
  );
}
