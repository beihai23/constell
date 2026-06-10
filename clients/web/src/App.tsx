import { BrowserRouter, Routes, Route, Navigate } from 'react-router';
import { ClientProvider } from '@/hooks/useConstellClient';
import { AuthGuard } from '@/components/auth/AuthGuard';
import LoginPage from '@/pages/LoginPage';
import RegisterPage from '@/pages/RegisterPage';
import MainPage from '@/pages/MainPage';
import { DMList } from '@/components/dm/DMList';
import { DMChat } from '@/components/chat/DMChat';
import { ChannelView } from '@/components/chat/ChannelView';
import { Toaster } from 'sonner';

export default function App() {
  return (
    <ClientProvider>
      <Toaster position="top-center" richColors />
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
            <Route path="/@me/:peerId" element={<DMChat />} />
            <Route path="/:communityId" element={<ChannelView />} />
            <Route path="/:communityId/:channelId" element={<ChannelView />} />
            <Route path="/" element={<Navigate to="/@me" replace />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </ClientProvider>
  );
}
