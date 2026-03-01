import { useState, useEffect, useRef, useMemo } from 'react';
import { v4 as uuidv4 } from 'uuid';

type PlayerType = 'Participant' | 'Observer';
type PlayerMode = 'Awake' | 'Asleep';

interface Player {
  publicId: number;
  name: string;
  type: PlayerType;
  mode: PlayerMode;
}

interface PokerServer {
  id: string;
  players: Record<string, Player>;
  currentSession: {
    cardSet: string[];
    votes: Record<string, string>;
    isShown: boolean;
  };
}

interface LogMessage {
  user: string;
  message: string;
  timestamp: string;
}

interface ChatMessage {
  user: string;
  message: string;
  timestamp: string;
}

function App() {
  const [roomId, setRoomId] = useState<string | null>(() => {
    const path = window.location.pathname;
    const match = path.match(/\/room\/([a-f0-9-]{36})/);
    return match ? match[1] : null;
  });
  
  const [server, setServer] = useState<PokerServer | null>(null);
  const [playerName, setPlayerName] = useState(() => localStorage.getItem('playerName') || '');
  const [rememberName, setRememberName] = useState(() => !!localStorage.getItem('playerName'));
  const [playerType, setPlayerType] = useState<PlayerType>('Participant');
  const [currentPlayer, setCurrentPlayer] = useState<Player | null>(null);
  const [isInitializing, setIsInitializing] = useState(true);
  const [cardSet, setCardSet] = useState('1,2,3,5,8');
  const [logs, setLogs] = useState<LogMessage[]>([]);
  const [chats, setChats] = useState<ChatMessage[]>([]);
  const [chatInput, setChatInput] = useState('');
  const [notifications, setNotifications] = useState<{id: string, text: string, type: string}[]>([]);
  const [chosenCard, setChosenCard] = useState<string | null>(null);
  
  const socketRef = useRef<WebSocket | null>(null);
  const recoveryId = useRef<string>(localStorage.getItem('recoveryId') || uuidv4());
  const chatEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    localStorage.setItem('recoveryId', recoveryId.current);
  }, []);

  useEffect(() => {
    chatEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [chats]);

  useEffect(() => {
    const handlePopState = () => {
      const path = window.location.pathname;
      const match = path.match(/\/room\/([a-f0-9-]{36})/);
      setRoomId(match ? match[1] : null);
    };
    window.addEventListener('popstate', handlePopState);
    return () => window.removeEventListener('popstate', handlePopState);
  }, []);

  useEffect(() => {
    if (roomId) {
      connect();
    } else {
      setIsInitializing(false);
    }
  }, [roomId]);

  const addNotification = (text: string, type: string = 'info') => {
    const id = uuidv4();
    setNotifications(prev => [...prev, { id, text, type }]);
    setTimeout(() => {
      setNotifications(prev => prev.filter(n => n.id !== id));
    }, 5000);
  };

  const connect = () => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(`${protocol}//${window.location.host}/ws?roomId=${roomId}`);

    ws.onopen = () => {
      addNotification('Connected to server', 'success');
      setIsInitializing(false);
      
      // Auto-join if we have a player name
      const storedName = localStorage.getItem('playerName');
      if (storedName) {
        ws.send(JSON.stringify({
          action: 'join',
          payload: { 
            name: storedName, 
            recoveryId: recoveryId.current, 
            type: playerType 
          }
        }));
      }
    };

    ws.onmessage = (event) => {
      const msg = JSON.parse(event.data);
      switch(msg.type) {
        case 'join_success':
          setCurrentPlayer(msg.payload);
          break;
        case 'updated':
          setServer(msg.payload);
          if (msg.payload && currentPlayer) {
            const myVote = msg.payload.currentSession.votes[currentPlayer.publicId.toString()];
            if (myVote && !chosenCard) {
              setChosenCard(myVote);
            }
          }
          break;
        case 'log':
          setLogs(prev => [msg.payload, ...prev].slice(0, 20));
          break;
        case 'chat':
          setChats(prev => [...prev, msg.payload]);
          break;
        case 'kicked':
          setCurrentPlayer(null);
          setRoomId(null);
          window.history.pushState({}, '', '/');
          addNotification('You have been kicked from the room', 'danger');
          socketRef.current?.close();
          break;
        case 'clear':
          setChosenCard(null);
          addNotification('Votes cleared', 'warning');
          break;
      }
    };

    ws.onclose = () => {
      addNotification('Disconnected. Retrying...', 'danger');
      setTimeout(() => {
        if (roomId) connect();
      }, 3000);
    };

    socketRef.current = ws;
  };

  const createRoom = async () => {
    const res = await fetch('/api/create', {
      method: 'POST',
      body: JSON.stringify({ cardSet }),
      headers: { 'Content-Type': 'application/json' }
    });
    const data = await res.json();
    window.history.pushState({}, '', `/room/${data.id}`);
    setRoomId(data.id);
  };

  const join = () => {
    if (rememberName) {
      localStorage.setItem('playerName', playerName);
    } else {
      localStorage.removeItem('playerName');
    }
    socketRef.current?.send(JSON.stringify({
      action: 'join',
      payload: { 
        name: playerName, 
        recoveryId: recoveryId.current, 
        type: playerType 
      }
    }));
  };

  const leave = () => {
    socketRef.current?.send(JSON.stringify({ action: 'leave' }));
    setCurrentPlayer(null);
    setRoomId(null);
    window.history.pushState({}, '', '/');
  };

  const vote = (card: string) => {
    if (server?.currentSession.isShown) return;
    if (chosenCard === card) {
      setChosenCard(null);
      socketRef.current?.send(JSON.stringify({ action: 'unvote' }));
    } else {
      setChosenCard(card);
      socketRef.current?.send(JSON.stringify({ action: 'vote', payload: { vote: card } }));
    }
  };

  const show = () => socketRef.current?.send(JSON.stringify({ action: 'show' }));
  const clear = () => socketRef.current?.send(JSON.stringify({ action: 'clear' }));
  const kick = (publicId: number) => socketRef.current?.send(JSON.stringify({ action: 'kick', payload: { publicId } }));
  const changeType = (type: PlayerType) => socketRef.current?.send(JSON.stringify({ action: 'changeType', payload: { type } }));
  
  const sendChat = (e?: React.FormEvent) => {
    e?.preventDefault();
    if (!chatInput.trim()) return;
    socketRef.current?.send(JSON.stringify({ action: 'chat', payload: { message: chatInput } }));
    setChatInput('');
  };

  const copyUrl = () => {
    navigator.clipboard.writeText(window.location.href);
    addNotification('URL copied to clipboard', 'success');
  };

  const voteStats = useMemo(() => {
    if (!server?.currentSession.isShown || !server.currentSession.votes) return null;
    const votes = Object.values(server.currentSession.votes).map(v => parseFloat(v)).filter(v => !isNaN(v));
    if (votes.length === 0) return null;
    const avg = votes.reduce((a, b) => a + b, 0) / votes.length;
    
    const counts: Record<string, number> = {};
    Object.values(server.currentSession.votes).forEach(v => counts[v] = (counts[v] || 0) + 1);
    const maxCount = Math.max(...Object.values(counts));
    const modes = Object.keys(counts).filter(k => counts[k] === maxCount);

    return { avg, modes };
  }, [server]);

  if (isInitializing && roomId) {
    return (
      <div className="min-h-screen d-flex align-items-center justify-content-center flex-column">
        <div className="spinner-border text-primary mb-3" role="status"></div>
        <div className="h5 text-uppercase tracking-wider">Initializing session...</div>
      </div>
    );
  }

  return (
    <>
      <nav className="navbar navbar-expand-lg">
        <div className="container-fluid px-4">
          <a className="navbar-brand" href="/">
             <i className="material-icons mr-2 text-primary" style={{verticalAlign: 'middle'}}>style</i>
             Planning Poker
          </a>
          <div className="ml-auto d-flex align-items-center">
            {playerName && (
              <button className="btn btn-link text-muted btn-sm mr-3 p-0" onClick={() => {
                setPlayerName('');
                localStorage.removeItem('playerName');
                setCurrentPlayer(null);
              }}>
                <span className="oi oi-person mr-1"></span> {playerName} (Change)
              </button>
            )}
            {roomId && currentPlayer && (
              <>
                <button className="btn btn-outline-info btn-sm mr-2" onClick={copyUrl}>
                  <span className="oi oi-share mr-1"></span> Share
                </button>
                <button className="btn btn-outline-danger btn-sm" onClick={leave}>
                  <span className="oi oi-account-logout mr-1"></span> Exit Room
                </button>
              </>
            )}
          </div>
        </div>
      </nav>

      <div className="container-fluid px-4 pb-5">
        {!roomId ? (
          <div className="container">
            <div className="card shadow-lg mt-5 border-0">
              <div className="card-body p-5">
                <h2 className="mb-3 font-weight-bold">Planning Poker</h2>
                <p className="lead text-muted">Estimate your tasks with zero friction. No accounts, no data tracking, just collaborative refinement.</p>
                <div className="form-group mt-5">
                  <label className="font-weight-bold">Card Set Configuration</label>
                  <input className="form-control form-control-lg" value={cardSet} onChange={e => setCardSet(e.target.value)} />
                  <small className="form-text text-muted">Customise the deck using comma-separated values.</small>
                </div>
                <button className="btn btn-primary btn-lg px-5 mt-4" onClick={createRoom}>Create Room</button>
              </div>
            </div>
          </div>
        ) : !currentPlayer ? (
          <div className="card shadow-lg mx-auto border-0" style={{maxWidth: '500px'}}>
            <div className="card-body p-4">
              <h4 className="mb-4">Join Session</h4>
              <p className="text-muted">Pick a username, and begin planning!</p>
              <form onSubmit={(e) => { e.preventDefault(); join(); }}>
                <div className="form-group">
                  <label>Username</label>
                  <input className="form-control" maxLength={20} value={playerName} onChange={e => setPlayerName(e.target.value)} />
                </div>
                <div className="custom-control custom-checkbox mb-3">
                  <input type="checkbox" className="custom-control-input" id="rememberName" checked={rememberName} onChange={e => setRememberName(e.target.checked)} />
                  <label className="custom-control-label text-muted" htmlFor="rememberName" style={{fontSize: '0.9rem'}}>Remember me on this device</label>
                </div>
                <div className="form-group">
                  <label>Participation type</label>
                  <select className="form-control custom-select" value={playerType} onChange={e => setPlayerType(e.target.value as PlayerType)}>
                    <option value="Participant">Participant</option>
                    <option value="Observer">Observer</option>
                  </select>
                </div>
                <button type="submit" className="btn btn-primary btn-block btn-lg mt-4" disabled={!playerName}>Join</button>
              </form>
            </div>
          </div>
        ) : (
          <div className="row">
            {/* Left Column: People, Log, Chat (rearranged to 2 major cols) */}
            <div className="col-lg-8">
              {/* Poker Cards */}
              <div className="card shadow-sm mb-4">
                <div className="card-body">
                  <h5 className="card-title font-weight-bold">Poker cards</h5>
                  <h6 className="card-subtitle mb-3 text-muted">Click on a card to cast your vote</h6>
                                        <div className="d-flex flex-wrap justify-content-center">
                                      {server?.currentSession.cardSet.map(card => (
                                        <button 
                                          key={card} 
                                          className={`btn poker_card ${chosenCard === card ? 'selected' : ''}`}
                                          onClick={() => vote(card)}
                                          disabled={currentPlayer.type === 'Observer' || server?.currentSession.isShown}
                                        >
                                          {card}
                                        </button>
                                      ))}
                                    </div>                </div>
              </div>

              <div className="row">
                <div className="col-md-6">
                  <div className="card shadow-sm h-100">
                    <div className="card-body">
                      <h6 className="font-weight-bold">Session Controls</h6>
                      <div className="row mt-3">
                        <div className="col-6">
                          <button className="btn btn-outline-primary btn-block btn-sm" onClick={clear}>Clear</button>
                        </div>
                        <div className="col-6">
                          <button className="btn btn-primary btn-block btn-sm" onClick={show}>Show</button>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
                <div className="col-md-6">
                  <div className="card shadow-sm h-100">
                    <div className="card-body">
                      <h6 className="font-weight-bold">Results</h6>
                      {server?.currentSession.isShown ? (
                        <div className="mt-2">
                          <div className="d-flex justify-content-between align-items-center mb-2">
                            <span className="text-muted">Avg:</span>
                            <span className="font-weight-bold" style={{fontSize: '2rem', color: 'var(--success-color)'}}>
                              {voteStats ? (voteStats.avg % 1 === 0 ? voteStats.avg : voteStats.avg.toFixed(1)) : '-'}
                            </span>
                          </div>
                          <div className="d-flex justify-content-between align-items-center">
                            <span className="text-muted">Mode:</span>
                            <span className="font-weight-bold" style={{fontSize: '1.5rem'}}>{voteStats?.modes.join(', ') || '-'}</span>
                          </div>
                        </div>
                      ) : (
                        <div className="text-center py-4 text-muted small italic">Votes hidden</div>
                      )}
                    </div>
                  </div>
                </div>
              </div>

              {/* Participants Section */}
              <div className="card shadow-sm mt-4">
                <div className="card-body p-3">
                  <h6 className="font-weight-bold mb-3">Participants</h6>
                  <div className="table-responsive">
                    <table className="table table-sm table-striped mb-0">
                      <thead>
                        <tr>
                          <th style={{width: '40px'}}></th>
                          <th>Name</th>
                          <th>Vote</th>
                          <th style={{width: '80px'}}></th>
                        </tr>
                      </thead>
                      <tbody>
                        {Object.values(server?.players || {})
                          .filter(p => p.type === 'Participant')
                          .sort((a,b) => a.publicId - b.publicId)
                          .map(p => {
                            const hasVoted = server?.currentSession.votes[p.publicId];
                            return (
                              <tr key={p.publicId} className={`${p.mode === 'Asleep' ? 'asleep' : ''} ${hasVoted ? 'table-success' : ''}`}>
                                <td>
                                  {hasVoted && p.mode === 'Awake' && <span className="oi oi-check text-success"></span>}
                                  {p.mode === 'Asleep' && <span className="oi oi-moon"></span>}
                                </td>
                                <td className="small font-weight-bold">{p.name}</td>
                                <td className="small">
                                  {server?.currentSession.isShown ? (hasVoted || '-') : (hasVoted ? '✅' : '-')}
                                </td>
                                <td className="text-right">
                                  {p.publicId === currentPlayer.publicId && (
                                    <button className="btn btn-link changetype-btn p-0 mr-2" 
                                            title="Change to Observer"
                                            onClick={() => changeType('Observer')}>
                                      <span className="oi oi-loop"></span>
                                    </button>
                                  )}
                                  <button className="btn btn-link kick-btn p-0" onClick={() => kick(p.publicId)}>
                                    <span className="oi oi-x"></span>
                                  </button>
                                </td>
                              </tr>
                            );
                          })}
                      </tbody>
                    </table>
                  </div>
                </div>
              </div>

              {/* Observers Section */}
              {Object.values(server?.players || {}).some(p => p.type === 'Observer') && (
                <div className="card shadow-sm mt-4">
                  <div className="card-body p-3">
                    <h6 className="font-weight-bold mb-3">Observers</h6>
                    <div className="table-responsive">
                      <table className="table table-sm table-striped mb-0">
                        <thead>
                          <tr>
                            <th style={{width: '40px'}}></th>
                            <th>Name</th>
                            <th style={{width: '80px'}}></th>
                          </tr>
                        </thead>
                        <tbody>
                          {Object.values(server?.players || {})
                            .filter(p => p.type === 'Observer')
                            .sort((a,b) => a.publicId - b.publicId)
                            .map(p => (
                              <tr key={p.publicId} className={p.mode === 'Asleep' ? 'asleep' : ''}>
                                <td>
                                  {p.mode === 'Asleep' && <span className="oi oi-moon"></span>}
                                </td>
                                <td className="small font-weight-bold">{p.name}</td>
                                <td className="text-right">
                                  {p.publicId === currentPlayer.publicId && (
                                    <button className="btn btn-link changetype-btn p-0 mr-2" 
                                            title="Change to Participant"
                                            onClick={() => changeType('Participant')}>
                                      <span className="oi oi-loop"></span>
                                    </button>
                                  )}
                                  <button className="btn btn-link kick-btn p-0" onClick={() => kick(p.publicId)}>
                                    <span className="oi oi-x"></span>
                                  </button>
                                </td>
                              </tr>
                            ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                </div>
              )}

              {/* Activity Log (Now below People) */}
              <div className="card shadow-sm mt-4">
                <div className="card-body p-3">
                  <h6 className="font-weight-bold mb-2">Activity Log</h6>
                  <div style={{maxHeight: '200px', overflowY: 'auto'}} className="pr-2">
                    {logs.map((l, i) => (
                      <div key={i} className="mb-2 pb-1 border-bottom border-light" style={{fontSize: '0.75rem'}}>
                        <span className="font-weight-bold" style={{color: 'var(--primary-color)'}}>{l.user}</span>
                        <span className="ml-2 text-muted">{l.message}</span>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            </div>

            {/* Right Column: Chat */}
            <div className="col-lg-4">
              <div className="card shadow-sm d-flex flex-column chat-panel">
                <div className="card-header bg-transparent border-bottom">
                  <h6 className="mb-0 font-weight-bold">Chat</h6>
                </div>
                <div className="card-body d-flex flex-column overflow-auto p-3 flex-grow-1" style={{background: 'rgba(0,0,0,0.02)', minHeight: 0, flex: '1 1 0'}}>
                  {chats.map((c, i) => (
                    <div key={i} className={`d-flex flex-column ${c.user === playerName ? 'align-items-end mine' : 'align-items-start'}`}>
                      <div className="chat-user-label">{c.user} • {new Date(c.timestamp).toLocaleTimeString([], {hour: '2-digit', minute:'2-digit'})}</div>
                      <div className={`chat-bubble ${c.user === playerName ? 'mine' : 'theirs'}`}>
                        {c.message.split(' ').map((word, j) => 
                          word.startsWith('http') ? <a key={j} href={word} target="_blank" rel="noopener noreferrer" style={{color: 'inherit', textDecoration: 'underline'}}>{word} </a> : word + ' '
                        )}
                      </div>
                    </div>
                  ))}
                  <div ref={chatEndRef} />
                </div>
                <div className="card-footer bg-transparent border-top p-2">
                  <form onSubmit={sendChat} className="input-group">
                    <input 
                      className="form-control form-control-sm" 
                      placeholder="Type a message..." 
                      value={chatInput}
                      onChange={e => setChatInput(e.target.value)}
                    />
                    <div className="input-group-append">
                      <button className="btn btn-primary btn-sm" type="submit">
                        <span className="oi oi-share-accessible mr-1"></span> Send
                      </button>
                    </div>
                  </form>
                </div>
              </div>
            </div>
          </div>
        )}
      </div>

      <div className="notifications">
        {notifications.map(n => (
          <div key={n.id} className={`notification alert-${n.type}`}>
            <span className={`oi oi-${n.type === 'success' ? 'check' : n.type === 'danger' ? 'warning' : 'info'} mr-2`}></span>
            {n.text}
          </div>
        ))}
      </div>
    </>
  );
}

export default App;
