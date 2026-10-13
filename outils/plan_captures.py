import json
MENU={'afterburner':'1','barbarian-2':'1','barbarian':'3','buggy-boy':'1','captain-blood':'1','castle-master':'2',
 'crazy-cars-2':'1','dungeon-master':'1','golden-axe':'1','light-corridor':'2','lotus-turbo':'2','midwinter':'1',
 'pirates':'1','populous':'1','r-type':'1','speedball':'2','xenon':'5'}
MENU_APRES_INTRO={'chase-hq':'1','kick-off-2':'1','prince-of-persia':'1','new-zealand-story':'1','turrican-2':'1'}
AUTO={'ik-plus':'A:\\IK+.PRG','monkey-island':'A:\\MONKEY.PRG','mortevielle':'A:\\MORT.PRG',
 'robocop':'A:\\ROBOCOP\\ROBOBOY.PRG','super-hang-on':'A:\\HANG_ON.PRG','their-finest-hour':'A:\\BOB.PRG',
 'voyageurs-du-temps':'A:\\DELPHINE.PRG','zak-mckracken':'A:\\ZAK.PRG','defender-of-the-crown':'A:\\ALESTART.TOS'}
OK={'bubble-bobble','chambers-of-shaolin','north-and-south','panza-kick-boxing','paperboy','winter-games','stunt-car-racer'}
TOS={'loom':('tos/emutos-2014.img','st')}
if __name__=='__main__':
  c=json.load(open('/home/claude/pack/catalogue.json'))
  U="/mnt/user-data/uploads/Hatari Loader/"
  with open('/tmp/jobs2.txt','w') as f:
    for j in c['jeux']:
      i=j['id']
      if i in OK: continue
      d=j['variantes'][0]['disques']
      tos,m=TOS.get(i,('tos/tos162fr.img','ste'))
      if i in MENU: k='18:%s 34:space 46:space'%MENU[i]
      elif i in MENU_APRES_INTRO: k='16:space 24:%s 36:space 48:space'%MENU_APRES_INTRO[i]
      else: k='22:space 32:space 42:space 54:space'
      f.write('\t'.join([i,U+tos,m,U+d[0],U+d[1] if len(d)>1 else '-',AUTO.get(i,'-'),k])+'\n')
